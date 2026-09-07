package repository

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// ─── Invitations ──────────────────────────────────────────────────────────────

// InviteCandidates adds candidates to a drive. Re-inviting an existing email is
// a no-op rather than an error, so a recruiter can safely paste an overlapping
// list twice.
func (r *Repo) InviteCandidates(ctx context.Context, req *assessmentv1.InviteCandidatesRequest) (*assessmentv1.InviteCandidatesResponse, error) {
	out := &assessmentv1.InviteCandidatesResponse{Invites: []*assessmentv1.Invite{}}
	seen := map[string]bool{}

	for _, raw := range req.Emails {
		email := strings.ToLower(strings.TrimSpace(raw))
		if email == "" || !strings.Contains(email, "@") || seen[email] {
			out.Skipped++
			continue
		}
		seen[email] = true

		token, err := inviteToken()
		if err != nil {
			return nil, err
		}

		inv := &assessmentv1.Invite{AssessmentId: req.AssessmentId, Email: email}
		var expires *time.Time
		err = r.pool.QueryRow(ctx, `
			INSERT INTO assessment_invites (assessment_id, email, token, expires_at, sent_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (assessment_id, email) DO UPDATE SET expires_at = EXCLUDED.expires_at
			RETURNING id::text, token, status, expires_at
		`, req.AssessmentId, email, token, parseTime(req.ExpiresAt)).
			Scan(&inv.Id, &inv.Token, &inv.Status, &expires)
		if err != nil {
			return nil, fmt.Errorf("insert invite: %w", err)
		}
		inv.ExpiresAt = fmtTime(expires)
		out.Invites = append(out.Invites, inv)
	}
	return out, nil
}

// ListInvites shows the recruiter who has been invited and how far they got.
func (r *Repo) ListInvites(ctx context.Context, assessmentID string) (*assessmentv1.ListInvitesResponse, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, email, COALESCE(user_id::text, ''), token, status, expires_at, sent_at
		FROM   assessment_invites WHERE assessment_id = $1 ORDER BY email
	`, assessmentID)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()

	out := &assessmentv1.ListInvitesResponse{Invites: []*assessmentv1.Invite{}}
	for rows.Next() {
		inv := &assessmentv1.Invite{AssessmentId: assessmentID}
		var expires, sent *time.Time
		if err := rows.Scan(&inv.Id, &inv.Email, &inv.UserId, &inv.Token, &inv.Status, &expires, &sent); err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		inv.ExpiresAt, inv.SentAt = fmtTime(expires), fmtTime(sent)
		out.Invites = append(out.Invites, inv)
	}
	return out, rows.Err()
}

func inviteToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate invite token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// ─── Recruiter reporting ──────────────────────────────────────────────────────

// ListAttempts is the recruiter's candidate table: filterable, sortable and
// paginated.
func (r *Repo) ListAttempts(ctx context.Context, req *assessmentv1.ListAttemptsRequest) (*assessmentv1.ListAttemptsResponse, error) {
	page, pageSize, offset := clampPage(req.Page, req.PageSize)

	clauses := []string{"at.assessment_id = $1"}
	args := []any{req.AssessmentId}
	add := func(clause string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if req.Status != "" {
		add("at.status = $%d", req.Status)
	}
	if req.MinScore > 0 {
		add("at.score >= $%d", req.MinScore)
	}
	if req.Search != "" {
		// One placeholder matched against both columns.
		args = append(args, req.Search)
		clauses = append(clauses, fmt.Sprintf(
			"(u.name ILIKE '%%' || $%d || '%%' OR u.email ILIKE '%%' || $%d || '%%')", len(args), len(args)))
	}
	where := "WHERE " + strings.Join(clauses, " AND ")

	var total int32
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM attempts at LEFT JOIN users u ON u.id = at.user_id `+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count attempts: %w", err)
	}

	summaries, err := r.attemptSummaries(ctx, where, args, orderClause(req.SortBy, req.SortDir), pageSize, offset)
	if err != nil {
		return nil, err
	}
	return &assessmentv1.ListAttemptsResponse{
		Attempts: summaries,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// orderClause maps the client's sort key onto a whitelisted column. Anything
// unrecognized falls back to score — the sort key is never interpolated raw.
func orderClause(sortBy, sortDir string) string {
	col := "at.score"
	switch sortBy {
	case "submitted_at":
		col = "at.submitted_at"
	case "integrity":
		col = "at.integrity_score"
	case "name":
		col = "u.name"
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	return col + " " + dir + " NULLS LAST, at.started_at DESC"
}

// GetAttemptReport is the recruiter's answer-by-answer view, including the
// answer key, submitted code and the full proctoring ledger.
func (r *Repo) GetAttemptReport(ctx context.Context, attemptID string) (*assessmentv1.AttemptReport, error) {
	summary, err := r.AttemptSummary(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	questions, err := r.attemptQuestions(ctx, attemptID, true)
	if err != nil {
		return nil, err
	}
	events, err := r.proctorEvents(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	return &assessmentv1.AttemptReport{Summary: summary, Questions: questions, ProctorEvents: events}, nil
}

func (r *Repo) proctorEvents(ctx context.Context, attemptID string) ([]*assessmentv1.ProctorEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT kind, detail, occurred_at FROM proctor_events
		WHERE attempt_id = $1 ORDER BY occurred_at
	`, attemptID)
	if err != nil {
		return nil, fmt.Errorf("load proctor events: %w", err)
	}
	defer rows.Close()

	out := []*assessmentv1.ProctorEvent{}
	for rows.Next() {
		e := &assessmentv1.ProctorEvent{}
		var at time.Time
		if err := rows.Scan(&e.Kind, &e.Detail, &at); err != nil {
			return nil, fmt.Errorf("scan proctor event: %w", err)
		}
		e.OccurredAt = fmtTime(&at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ─── Shortlisting ─────────────────────────────────────────────────────────────

// ComputeShortlist ranks the candidate pool against the recruiter's criteria.
//
// With Save false it returns a preview only, so criteria can be tuned before
// anything is committed — which is how a recruiter actually works: adjust the
// cutoff, watch the count move, then save.
func (r *Repo) ComputeShortlist(ctx context.Context, req *assessmentv1.ComputeShortlistRequest) (*assessmentv1.Shortlist, error) {
	criteria := req.Criteria
	if criteria == nil {
		criteria = &assessmentv1.ShortlistCriteria{}
	}

	// Only finished attempts are rankable; one still evaluating has no final
	// score and must not silently rank below everyone.
	summaries, err := r.attemptSummaries(ctx,
		`WHERE at.assessment_id = $1 AND at.status IN ('evaluated', 'disqualified')`,
		[]any{req.AssessmentId}, "at.score DESC", 0, 0)
	if err != nil {
		return nil, err
	}

	eligible := make([]*assessmentv1.AttemptSummary, 0, len(summaries))
	for _, s := range summaries {
		if !meetsCriteria(s, criteria) {
			continue
		}
		eligible = append(eligible, s)
	}

	// Ties break on integrity, then on finishing earlier — both reward the
	// candidate who did the same work more cleanly or faster.
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Score != eligible[j].Score {
			return eligible[i].Score > eligible[j].Score
		}
		if eligible[i].IntegrityScore != eligible[j].IntegrityScore {
			return eligible[i].IntegrityScore > eligible[j].IntegrityScore
		}
		return eligible[i].SubmittedAt < eligible[j].SubmittedAt
	})

	if criteria.TopN > 0 && int(criteria.TopN) < len(eligible) {
		eligible = eligible[:criteria.TopN]
	}

	out := &assessmentv1.Shortlist{
		AssessmentId: req.AssessmentId,
		Name:         defaultStr(req.Name, "Shortlist"),
		Criteria:     criteria,
		Entries:      make([]*assessmentv1.ShortlistEntry, 0, len(eligible)),
		Total:        int32(len(eligible)),
	}
	for i, s := range eligible {
		out.Entries = append(out.Entries, &assessmentv1.ShortlistEntry{
			AttemptId: s.Id,
			UserId:    s.UserId,
			UserName:  s.UserName,
			UserEmail: s.UserEmail,
			Rank:      int32(i + 1),
			Score:     s.Score,
			MaxScore:  s.MaxScore,
			Percent:   s.Percent,
			Integrity: s.IntegrityScore,
			Decision:  "shortlisted",
		})
	}

	if !req.Save {
		return out, nil
	}
	if err := r.saveShortlist(ctx, req, out); err != nil {
		return nil, err
	}
	return out, nil
}

// meetsCriteria applies the recruiter's cut rules to one attempt.
func meetsCriteria(s *assessmentv1.AttemptSummary, c *assessmentv1.ShortlistCriteria) bool {
	if c.ExcludeFlagged && (s.Status == "disqualified" || s.IntegrityScore < 100) {
		return false
	}
	if c.MinScorePercent > 0 && s.Percent < c.MinScorePercent {
		return false
	}
	if c.MinIntegrity > 0 && s.IntegrityScore < c.MinIntegrity {
		return false
	}
	for sectionID, cutoff := range c.SectionCutoffs {
		if s.SectionScores[sectionID] < cutoff {
			return false
		}
	}
	return true
}

func (r *Repo) saveShortlist(ctx context.Context, req *assessmentv1.ComputeShortlistRequest, out *assessmentv1.Shortlist) error {
	criteriaJSON, err := json.Marshal(out.Criteria)
	if err != nil {
		return fmt.Errorf("marshal criteria: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save shortlist: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var id string
	var created time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO shortlists (assessment_id, name, criteria, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, created_at
	`, req.AssessmentId, out.Name, criteriaJSON, req.ActorId).Scan(&id, &created); err != nil {
		return fmt.Errorf("insert shortlist: %w", err)
	}

	for _, e := range out.Entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO shortlist_entries (shortlist_id, attempt_id, user_id, rank)
			VALUES ($1, $2, $3, $4)
		`, id, e.AttemptId, e.UserId, e.Rank); err != nil {
			return fmt.Errorf("insert shortlist entry: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit save shortlist: %w", err)
	}

	out.Id, out.CreatedBy, out.CreatedAt = id, req.ActorId, fmtTime(&created)
	return nil
}

// ListShortlists returns saved shortlists for a drive, newest first.
func (r *Repo) ListShortlists(ctx context.Context, assessmentID string) (*assessmentv1.ListShortlistsResponse, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id::text, s.name, s.criteria, s.created_by::text, s.created_at,
		       (SELECT COUNT(*) FROM shortlist_entries e WHERE e.shortlist_id = s.id)
		FROM   shortlists s WHERE s.assessment_id = $1 ORDER BY s.created_at DESC
	`, assessmentID)
	if err != nil {
		return nil, fmt.Errorf("list shortlists: %w", err)
	}
	defer rows.Close()

	out := &assessmentv1.ListShortlistsResponse{Shortlists: []*assessmentv1.Shortlist{}}
	for rows.Next() {
		s := &assessmentv1.Shortlist{AssessmentId: assessmentID, Criteria: &assessmentv1.ShortlistCriteria{}}
		var criteria []byte
		var created time.Time
		if err := rows.Scan(&s.Id, &s.Name, &criteria, &s.CreatedBy, &created, &s.Total); err != nil {
			return nil, fmt.Errorf("scan shortlist: %w", err)
		}
		fromJSON(criteria, s.Criteria)
		s.CreatedAt = fmtTime(&created)
		out.Shortlists = append(out.Shortlists, s)
	}
	return out, rows.Err()
}

// GetShortlist loads a saved shortlist with its ranked entries and current
// per-candidate decisions.
func (r *Repo) GetShortlist(ctx context.Context, id string) (*assessmentv1.Shortlist, error) {
	s := &assessmentv1.Shortlist{Id: id, Criteria: &assessmentv1.ShortlistCriteria{}, Entries: []*assessmentv1.ShortlistEntry{}}
	var criteria []byte
	var created time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT assessment_id::text, name, criteria, created_by::text, created_at
		FROM   shortlists WHERE id = $1
	`, id).Scan(&s.AssessmentId, &s.Name, &criteria, &s.CreatedBy, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get shortlist: %w", err)
	}
	fromJSON(criteria, s.Criteria)
	s.CreatedAt = fmtTime(&created)

	rows, err := r.pool.Query(ctx, `
		SELECT e.attempt_id::text, e.user_id::text, COALESCE(u.name, ''), COALESCE(u.email, ''),
		       e.rank, e.decision, e.notes,
		       at.score, at.max_score, at.integrity_score
		FROM   shortlist_entries e
		JOIN   attempts at ON at.id = e.attempt_id
		LEFT   JOIN users u ON u.id = e.user_id
		WHERE  e.shortlist_id = $1
		ORDER  BY e.rank
	`, id)
	if err != nil {
		return nil, fmt.Errorf("load shortlist entries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		e := &assessmentv1.ShortlistEntry{}
		if err := rows.Scan(&e.AttemptId, &e.UserId, &e.UserName, &e.UserEmail,
			&e.Rank, &e.Decision, &e.Notes, &e.Score, &e.MaxScore, &e.Integrity); err != nil {
			return nil, fmt.Errorf("scan shortlist entry: %w", err)
		}
		if e.MaxScore > 0 {
			e.Percent = round2(e.Score / e.MaxScore * 100)
		}
		s.Entries = append(s.Entries, e)
	}
	s.Total = int32(len(s.Entries))
	return s, rows.Err()
}

// SetCandidateDecision records the recruiter's call on one candidate.
func (r *Repo) SetCandidateDecision(ctx context.Context, req *assessmentv1.SetCandidateDecisionRequest) error {
	switch req.Decision {
	case "shortlisted", "rejected", "on_hold", "hired":
	default:
		return fmt.Errorf("decision must be shortlisted, rejected, on_hold or hired")
	}
	ct, err := r.pool.Exec(ctx, `
		UPDATE shortlist_entries SET decision = $3, notes = $4
		WHERE shortlist_id = $1 AND attempt_id = $2
	`, req.ShortlistId, req.AttemptId, req.Decision, req.Notes)
	if err != nil {
		return fmt.Errorf("set candidate decision: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ExportResults renders the candidate table as CSV. A shortlist id narrows the
// export to that shortlist, in rank order.
func (r *Repo) ExportResults(ctx context.Context, req *assessmentv1.ExportResultsRequest) (*assessmentv1.ExportResultsResponse, error) {
	var title string
	if err := r.pool.QueryRow(ctx, `SELECT title FROM assessments WHERE id = $1`, req.AssessmentId).Scan(&title); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lookup assessment title: %w", err)
	}

	var summaries []*assessmentv1.AttemptSummary
	var err error
	if req.ShortlistId != "" {
		summaries, err = r.attemptSummaries(ctx, `
			WHERE at.id IN (SELECT attempt_id FROM shortlist_entries WHERE shortlist_id = $1)`,
			[]any{req.ShortlistId}, "at.score DESC", 0, 0)
	} else {
		summaries, err = r.attemptSummaries(ctx,
			`WHERE at.assessment_id = $1`, []any{req.AssessmentId}, "at.score DESC", 0, 0)
	}
	if err != nil {
		return nil, err
	}

	var buf strings.Builder
	w := csv.NewWriter(&buf)
	header := []string{"Rank", "Name", "Email", "Status", "Score", "Max Score",
		"Percent", "Passed", "Integrity", "Decision", "Started", "Submitted"}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("write csv header: %w", err)
	}

	for i, s := range summaries {
		row := []string{
			strconv.Itoa(i + 1),
			s.UserName,
			s.UserEmail,
			s.Status,
			strconv.FormatFloat(s.Score, 'f', 2, 64),
			strconv.FormatFloat(s.MaxScore, 'f', 2, 64),
			strconv.FormatFloat(s.Percent, 'f', 2, 64),
			strconv.FormatBool(s.Passed),
			strconv.FormatFloat(s.IntegrityScore, 'f', 0, 64),
			s.Decision,
			s.StartedAt,
			s.SubmittedAt,
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("write csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flush csv: %w", err)
	}

	return &assessmentv1.ExportResultsResponse{
		Filename: slugify(title) + "-results.csv",
		Csv:      buf.String(),
	}, nil
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "assessment"
	}
	return out
}
