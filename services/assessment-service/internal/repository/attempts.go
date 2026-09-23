package repository

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/knovate211/assessment-service/internal/grading"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// ErrAttemptClosed is returned by every write path once an attempt is no
// longer live. Callers surface it as "your test has ended".
var ErrAttemptClosed = errors.New("attempt is no longer open")

// ErrNoAttemptsLeft is returned when the candidate has used every allowed try.
var ErrNoAttemptsLeft = errors.New("no attempts remaining")

// ErrNotInvited guards hiring drives: those are invite-only.
var ErrNotInvited = errors.New("you are not invited to this assessment")

// querier is the subset of pgx shared by *pgxpool.Pool and pgx.Tx, so scoring
// helpers can run either inside a transaction or standalone.
type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// attemptCtx is the attempt joined with the assessment settings that govern it.
type attemptCtx struct {
	ID              string
	AssessmentID    string
	UserID          string
	Status          string
	Seed            int64
	StartedAt       time.Time
	ExpiresAt       time.Time
	MaxScore        float64
	Score           float64
	IntegrityScore  float64
	Title           string
	AllowBacktrack  bool
	LockForward     bool
	RevealResults   bool
	NegativeMarking float64
	PassingMarks    float64
	Purpose         string
	Proctoring      *assessmentv1.Proctoring
}

func (a *attemptCtx) secondsLeft(now time.Time) int64 {
	if a.Status != "in_progress" {
		return 0
	}
	left := int64(a.ExpiresAt.Sub(now).Seconds())
	if left < 0 {
		return 0
	}
	return left
}

// ─── Discovery ────────────────────────────────────────────────────────────────

// ListAvailableAssessments returns the tests a student can see: published
// practice tests plus hiring tests they hold an invite for.
func (r *Repo) ListAvailableAssessments(ctx context.Context, req *assessmentv1.ListAvailableAssessmentsRequest) (*assessmentv1.ListAvailableAssessmentsResponse, error) {
	email, err := r.userEmail(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.title, a.description, a.purpose,
		       COALESCE(c.name, ''), COALESCE(c.logo_url, ''),
		       a.duration_minutes, a.total_marks, a.opens_at, a.closes_at, a.max_attempts,
		       a.proctoring,
		       COALESCE(i.token, ''),
		       (SELECT COUNT(*) FROM attempts at
		         WHERE at.assessment_id = a.id AND at.user_id = $1),
		       COALESCE((SELECT at.id::text FROM attempts at
		         WHERE at.assessment_id = a.id AND at.user_id = $1
		           AND at.status = 'in_progress'
		         ORDER BY at.started_at DESC LIMIT 1), ''),
		       COALESCE((
		         SELECT string_agg(x.label, ' · ' ORDER BY x.order_index)
		         FROM (
		           SELECT s.order_index,
		                  COALESCE(s.pick_count, (SELECT COUNT(*) FROM section_questions sq WHERE sq.section_id = s.id))
		                  || ' ' || CASE s.kind
		                              WHEN 'mcq' THEN 'MCQ'
		                              ELSE initcap(s.kind)
		                            END AS label
		           FROM assessment_sections s WHERE s.assessment_id = a.id
		         ) x
		       ), ''),
		       COALESCE((
		         SELECT SUM(COALESCE(s.pick_count, (SELECT COUNT(*) FROM section_questions sq WHERE sq.section_id = s.id)))
		         FROM assessment_sections s WHERE s.assessment_id = a.id
		       ), 0)
		FROM   assessments a
		LEFT   JOIN companies c ON c.id = a.company_id
		LEFT   JOIN assessment_invites i
		       ON i.assessment_id = a.id AND lower(i.email) = lower($2)
		WHERE  a.status = 'published'
		  AND  (a.purpose = 'practice' OR i.id IS NOT NULL)
		  -- A practice test aimed at particular courses is shown only to
		  -- students enrolled in one of them (see practiceAudienceOK).
		  AND  (a.purpose <> 'practice' OR cardinality(a.course_ids) = 0
		        OR EXISTS (SELECT 1 FROM user_courses uc
		                   WHERE uc.user_id = $1 AND uc.course_id = ANY(a.course_ids)))
		ORDER  BY (a.purpose IN ('hiring', 'scholarship')) DESC, a.created_at DESC
	`, req.UserId, email)
	if err != nil {
		return nil, fmt.Errorf("list available assessments: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	out := &assessmentv1.ListAvailableAssessmentsResponse{Assessments: []*assessmentv1.AssessmentSummary{}}

	for rows.Next() {
		s := &assessmentv1.AssessmentSummary{}
		var opensAt, closesAt *time.Time
		var questionCount int64
		var proctoringRaw []byte
		if err := rows.Scan(&s.Id, &s.Title, &s.Description, &s.Purpose,
			&s.CompanyName, &s.CompanyLogo, &s.DurationMinutes, &s.TotalMarks,
			&opensAt, &closesAt, &s.MaxAttempts, &proctoringRaw, &s.InviteToken,
			&s.AttemptsUsed, &s.LiveAttemptId, &s.SectionSummary, &questionCount); err != nil {
			return nil, fmt.Errorf("scan available assessment: %w", err)
		}
		s.QuestionCount = int32(questionCount)
		if len(proctoringRaw) > 0 {
			var pr assessmentv1.Proctoring
			// A malformed blob must not hide the whole assessment from the
			// candidate; they simply see no proctoring notice.
			if err := json.Unmarshal(proctoringRaw, &pr); err == nil {
				s.Proctoring = &pr
			}
		}
		s.OpensAt, s.ClosesAt = fmtTime(opensAt), fmtTime(closesAt)
		s.CanStart, s.BlockedReason = startability(now, opensAt, closesAt, s)

		switch req.Scope {
		case "invited":
			if !inviteOnly(s.Purpose) {
				continue
			}
		case "completed":
			if s.AttemptsUsed == 0 {
				continue
			}
		}
		out.Assessments = append(out.Assessments, s)
	}
	return out, rows.Err()
}

// startability decides whether the Start button is live and, when it is not,
// why — the reason is shown to the candidate verbatim.
func startability(now time.Time, opensAt, closesAt *time.Time, s *assessmentv1.AssessmentSummary) (bool, string) {
	if s.LiveAttemptId != "" {
		return true, ""
	}
	if opensAt != nil && now.Before(*opensAt) {
		return false, "Opens " + opensAt.UTC().Format("2 Jan 2006, 15:04") + " UTC"
	}
	if closesAt != nil && now.After(*closesAt) {
		return false, "This test has closed"
	}
	if s.AttemptsUsed >= s.MaxAttempts {
		return false, "You have used all attempts"
	}
	return true, ""
}

func (r *Repo) userEmail(ctx context.Context, userID string) (string, error) {
	var email string
	err := r.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil // an unknown user simply matches no invites
	}
	if err != nil {
		return "", fmt.Errorf("lookup user email: %w", err)
	}
	return email, nil
}

// ─── Starting an attempt ──────────────────────────────────────────────────────

// StartAttempt materializes a paper for the candidate and starts the clock.
//
// It is idempotent: a candidate who refreshes, crashes or reconnects gets the
// same in-progress attempt back rather than a second one with a fresh timer.
func (r *Repo) StartAttempt(ctx context.Context, req *assessmentv1.StartAttemptRequest) (string, error) {
	// An existing live attempt always wins, before any eligibility checks —
	// otherwise a test that closed mid-attempt would lock the candidate out of
	// their own running paper.
	var liveID string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text FROM attempts
		WHERE  assessment_id = $1 AND user_id = $2 AND status = 'in_progress'
		ORDER  BY started_at DESC LIMIT 1
	`, req.AssessmentId, req.UserId).Scan(&liveID)
	if err == nil && liveID != "" {
		return liveID, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("check live attempt: %w", err)
	}

	a, err := r.GetAssessment(ctx, req.AssessmentId, true)
	if err != nil {
		return "", err
	}
	if a.Status != "published" {
		return "", fmt.Errorf("this test is not open")
	}

	now := time.Now().UTC()
	if opens := parseTimeValue(a.OpensAt); opens != nil && now.Before(*opens) {
		return "", fmt.Errorf("this test opens at %s", opens.Format(time.RFC1123))
	}
	if closes := parseTimeValue(a.ClosesAt); closes != nil && now.After(*closes) {
		return "", fmt.Errorf("this test has closed")
	}

	// Course audience for practice tests. The listing hides a test from
	// students outside its courses; this stops the same test being started by
	// id or a shared link.
	if err := r.practiceAudienceOK(ctx, a, req.UserId); err != nil {
		return "", err
	}

	// Invite check for hiring drives.
	inviteID, err := r.resolveInvite(ctx, a, req)
	if err != nil {
		return "", err
	}

	var used int32
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM attempts WHERE assessment_id = $1 AND user_id = $2`,
		req.AssessmentId, req.UserId).Scan(&used); err != nil {
		return "", fmt.Errorf("count attempts: %w", err)
	}
	if used >= maxInt32(a.MaxAttempts, 1) {
		return "", ErrNoAttemptsLeft
	}

	seed, err := randomSeed()
	if err != nil {
		return "", err
	}
	paper, err := r.buildPaper(ctx, a, seed)
	if err != nil {
		return "", err
	}
	if len(paper) == 0 {
		return "", fmt.Errorf("this test has no questions")
	}

	var maxScore float64
	for _, q := range paper {
		maxScore += q.Marks
	}

	// The deadline is set once, server-side, and is the only clock that counts.
	expiresAt := now.Add(time.Duration(a.DurationMinutes) * time.Minute)
	if closes := parseTimeValue(a.ClosesAt); closes != nil && closes.Before(expiresAt) {
		expiresAt = *closes
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin start attempt: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var attemptID string
	err = tx.QueryRow(ctx, `
		INSERT INTO attempts (assessment_id, user_id, invite_id, attempt_no, seed,
			started_at, expires_at, max_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text
	`, req.AssessmentId, req.UserId, nullable(inviteID), used+1, seed, now, expiresAt, maxScore).Scan(&attemptID)
	if err != nil {
		return "", fmt.Errorf("insert attempt: %w", err)
	}

	for _, q := range paper {
		if _, err := tx.Exec(ctx, `
			INSERT INTO attempt_questions (attempt_id, section_id, kind, mcq_question_id,
				problem_id, order_index, marks, option_order, grading_status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::uuid[], $9)
		`, attemptID, q.SectionID, q.Kind, nullable(q.McqQuestionID), nullable(q.ProblemID),
			q.OrderIndex, q.Marks, q.OptionOrder, initialGradingStatus(q.Kind)); err != nil {
			return "", fmt.Errorf("insert attempt question: %w", err)
		}
	}

	if inviteID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE assessment_invites SET status = 'started', user_id = $2 WHERE id = $1
		`, inviteID, req.UserId); err != nil {
			return "", fmt.Errorf("mark invite started: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit start attempt: %w", err)
	}
	return attemptID, nil
}

func initialGradingStatus(kind string) string {
	if kind == "descriptive" {
		return "manual_review"
	}
	return "ungraded"
}

// inviteOnly reports whether a purpose may only be attempted by someone holding
// an invite.
//
// This is the single definition of "who is allowed in", deliberately shared by
// resolveInvite and the candidate-facing listing so the two can never drift.
//
// It is written as a denylist of one rather than an allowlist of many, so that
// it fails closed: a purpose added to the assessments_purpose_check constraint
// but forgotten here becomes invite-gated, which surfaces as a candidate unable
// to start a paper. An allowlist fails the other way — the new purpose silently
// becomes open to every logged-in user, and nobody finds out until someone
// farms a paper they were never invited to. This also mirrors the SQL in
// ListAvailableAssessments, which already admits 'practice' and requires an
// invite for everything else.
// resultsWithheld reports whether the candidate is allowed to see their own
// score for this kind of paper.
//
// A scholarship result is not a test score, it is a fee decision: it is
// reviewed against the award ladder, can be adjusted by staff, and is delivered
// by email. Showing a raw percentage the moment the paper is submitted
// pre-empts that decision and — when coding answers are still grading — shows a
// zero that is simply wrong.
//
// Unlike inviteOnly this is an allowlist of one, and that is the right way
// round here: a purpose nobody has thought about should show results normally
// (the existing behaviour for practice and hiring), not silently stop showing
// them.
func resultsWithheld(purpose string) bool {
	return strings.EqualFold(strings.TrimSpace(purpose), "scholarship")
}

// practiceAudienceOK allows a practice test with no course list to everyone,
// and one with a course list only to students enrolled in one of its courses.
func (r *Repo) practiceAudienceOK(ctx context.Context, a *assessmentv1.Assessment, userID string) error {
	if inviteOnly(a.Purpose) || a.CourseIds == nil || len(*a.CourseIds) == 0 {
		return nil
	}
	var ok bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM user_courses WHERE user_id::text = $1 AND course_id = ANY($2))
	`, userID, *a.CourseIds).Scan(&ok); err != nil {
		return fmt.Errorf("check course enrolment: %w", err)
	}
	if !ok {
		return fmt.Errorf("this test is only open to students of its course")
	}
	return nil
}

func inviteOnly(purpose string) bool {
	return !strings.EqualFold(strings.TrimSpace(purpose), "practice")
}

// resolveInvite enforces invite-only access to hiring drives and scholarship
// papers, and returns the invite id to link to the attempt.
func (r *Repo) resolveInvite(ctx context.Context, a *assessmentv1.Assessment, req *assessmentv1.StartAttemptRequest) (string, error) {
	// A practice test needs no invite. Everything else does: the invite row is
	// the entire eligibility record, so returning early here for a scholarship
	// paper would let any authenticated student start one uninvited.
	if !inviteOnly(a.Purpose) {
		return "", nil
	}

	var id, status string
	var expires *time.Time
	var err error

	if req.InviteToken != "" {
		err = r.pool.QueryRow(ctx, `
			SELECT id::text, status, expires_at FROM assessment_invites
			WHERE token = $1 AND assessment_id = $2
		`, req.InviteToken, req.AssessmentId).Scan(&id, &status, &expires)
	} else {
		email, emailErr := r.userEmail(ctx, req.UserId)
		if emailErr != nil {
			return "", emailErr
		}
		if email == "" {
			return "", ErrNotInvited
		}
		err = r.pool.QueryRow(ctx, `
			SELECT id::text, status, expires_at FROM assessment_invites
			WHERE assessment_id = $1 AND lower(email) = lower($2)
		`, req.AssessmentId, email).Scan(&id, &status, &expires)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotInvited
	}
	if err != nil {
		return "", fmt.Errorf("resolve invite: %w", err)
	}
	if expires != nil && time.Now().UTC().After(*expires) {
		return "", fmt.Errorf("your invitation has expired")
	}
	return id, nil
}

// ─── Paper construction ───────────────────────────────────────────────────────

type paperQuestion struct {
	SectionID     string
	Kind          string
	McqQuestionID string
	ProblemID     string
	OrderIndex    int32
	Marks         float64
	// OptionOrder must never be nil. It maps to attempt_questions.option_order,
	// which is NOT NULL DEFAULT '{}' — and a column DEFAULT applies only when
	// the column is omitted from the INSERT, not when NULL is passed for it.
	// pgx encodes a nil []string as SQL NULL, so a nil here fails the insert.
	// Coding and descriptive questions have no options and would otherwise
	// leave it nil, which broke every mixed MCQ + coding paper.
	OptionOrder []string
}

// buildPaper draws and orders the questions for one attempt. Everything is
// derived from the attempt's seed, so the same seed reproduces the same paper
// exactly — which is what makes a disputed result reviewable.
func (r *Repo) buildPaper(ctx context.Context, a *assessmentv1.Assessment, seed int64) ([]*paperQuestion, error) {
	rng := mrand.New(mrand.NewSource(seed)) //nolint:gosec // deterministic by design, not security
	paper := []*paperQuestion{}
	order := int32(0)

	for _, s := range a.Sections {
		var picked []*paperQuestion

		switch {
		case len(s.Questions) > 0:
			for _, q := range s.Questions {
				picked = append(picked, &paperQuestion{
					SectionID:     s.Id,
					Kind:          s.Kind,
					McqQuestionID: q.McqQuestionId,
					ProblemID:     q.ProblemId,
					Marks:         float64(maxInt32(q.Marks, 1)),
					OptionOrder:   []string{},
				})
			}
			if s.PickCount > 0 && int(s.PickCount) < len(picked) {
				shuffle(rng, picked)
				picked = picked[:s.PickCount]
				// Drawn questions are equal-weight — see validateForPublish.
				for _, q := range picked {
					q.Marks = float64(maxInt32(s.PickMarks, 1))
				}
			}

		case s.PickCount > 0 && s.Kind == "mcq":
			ids, err := r.drawFromBank(ctx, a.CompanyId, s, rng)
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				picked = append(picked, &paperQuestion{
					SectionID:     s.Id,
					Kind:          s.Kind,
					McqQuestionID: id,
					Marks:         float64(maxInt32(s.PickMarks, 1)),
					OptionOrder:   []string{},
				})
			}
		}

		if a.ShuffleQuestions {
			shuffle(rng, picked)
		}
		for _, q := range picked {
			q.OrderIndex = order
			order++
		}
		paper = append(paper, picked...)
	}

	if a.ShuffleOptions {
		if err := r.assignOptionOrder(ctx, paper, rng); err != nil {
			return nil, err
		}
	} else if err := r.assignOptionOrder(ctx, paper, nil); err != nil {
		return nil, err
	}
	return paper, nil
}

// drawFromBank selects PickCount active bank questions matching the section's
// filter. Candidates are fetched in a stable order and shuffled with the
// attempt's seed so the draw stays reproducible.
func (r *Repo) drawFromBank(ctx context.Context, companyID string, s *assessmentv1.Section, rng *mrand.Rand) ([]string, error) {
	var clauses []string
	var args []any
	add := func(clause string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	clauses = append(clauses, "is_active = true")
	if companyID != "" {
		add("(company_id = $%d OR company_id IS NULL)", companyID)
	} else {
		clauses = append(clauses, "company_id IS NULL")
	}
	switch s.PickCourse {
	case "": // any course
	case assessmentv1.McqGeneralCourse:
		clauses = append(clauses, "course_id = ''")
	default:
		add("course_id = $%d", s.PickCourse)
	}
	if s.PickTopic != "" {
		add("topic = $%d", s.PickTopic)
	}
	if s.PickDifficulty != "" {
		add("difficulty = $%d", s.PickDifficulty)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id::text FROM mcq_questions
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY id
		LIMIT 2000
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("draw from bank: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan bank id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) < int(s.PickCount) {
		return nil, fmt.Errorf("section %q needs %d questions but the bank only has %d matching",
			s.Title, s.PickCount, len(ids))
	}

	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids[:s.PickCount], nil
}

// assignOptionOrder freezes the option sequence each candidate will see. When
// rng is nil the authored order is kept.
func (r *Repo) assignOptionOrder(ctx context.Context, paper []*paperQuestion, rng *mrand.Rand) error {
	ids := []string{}
	for _, q := range paper {
		if q.McqQuestionID != "" {
			ids = append(ids, q.McqQuestionID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT question_id::text, id::text
		FROM   mcq_options
		WHERE  question_id = ANY($1::uuid[])
		ORDER  BY question_id, order_index
	`, ids)
	if err != nil {
		return fmt.Errorf("load option order: %w", err)
	}
	defer rows.Close()

	byQuestion := map[string][]string{}
	for rows.Next() {
		var qid, oid string
		if err := rows.Scan(&qid, &oid); err != nil {
			return fmt.Errorf("scan option id: %w", err)
		}
		byQuestion[qid] = append(byQuestion[qid], oid)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, q := range paper {
		if q.McqQuestionID == "" {
			continue
		}
		// Starts from an empty slice, not nil: a question whose options failed
		// to load would otherwise re-introduce the nil this guards against.
		opts := append([]string{}, byQuestion[q.McqQuestionID]...)
		if rng != nil {
			rng.Shuffle(len(opts), func(i, j int) { opts[i], opts[j] = opts[j], opts[i] })
		}
		q.OptionOrder = opts
	}
	return nil
}

func shuffle(rng *mrand.Rand, qs []*paperQuestion) {
	rng.Shuffle(len(qs), func(i, j int) { qs[i], qs[j] = qs[j], qs[i] })
}

func randomSeed() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("generate attempt seed: %w", err)
	}
	// Keep it positive so it survives a BIGINT round trip unambiguously.
	return int64(binary.BigEndian.Uint64(b[:]) >> 1), nil
}

func parseTimeValue(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

// ─── Reading a live attempt ───────────────────────────────────────────────────

// loadAttempt fetches the attempt joined with the settings that govern it.
func (r *Repo) loadAttempt(ctx context.Context, q querier, attemptID string, lock bool) (*attemptCtx, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF at"
	}
	a := &attemptCtx{Proctoring: defaultProctoring()}
	var proctoring []byte
	err := q.QueryRow(ctx, `
		SELECT at.id::text, at.assessment_id::text, at.user_id::text, at.status, at.seed,
		       at.started_at, at.expires_at, at.max_score, at.score, at.integrity_score,
		       a.title, a.allow_backtrack, a.lock_forward, a.reveal_results, a.negative_marking,
		       a.passing_marks, a.purpose, a.proctoring
		FROM   attempts at
		JOIN   assessments a ON a.id = at.assessment_id
		WHERE  at.id = $1`+suffix, attemptID).
		Scan(&a.ID, &a.AssessmentID, &a.UserID, &a.Status, &a.Seed,
			&a.StartedAt, &a.ExpiresAt, &a.MaxScore, &a.Score, &a.IntegrityScore,
			&a.Title, &a.AllowBacktrack, &a.LockForward, &a.RevealResults, &a.NegativeMarking,
			&a.PassingMarks, &a.Purpose, &proctoring)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load attempt: %w", err)
	}
	fromJSON(proctoring, a.Proctoring)
	return a, nil
}

// GetAttemptState returns the full paper and saved answers for the player.
//
// It is the resume path: the server holds every byte of attempt state, so a
// refresh, a crash or a new device picks up exactly where the candidate left
// off. It also enforces the deadline — a stale client that keeps polling past
// expiry gets its attempt auto-submitted here.
func (r *Repo) GetAttemptState(ctx context.Context, attemptID, userID string) (*assessmentv1.AttemptState, error) {
	a, err := r.loadAttempt(ctx, r.pool, attemptID, false)
	if err != nil {
		return nil, err
	}
	if a.UserID != userID {
		return nil, ErrForbidden
	}

	now := time.Now().UTC()
	if a.Status == "in_progress" && now.After(a.ExpiresAt) {
		if _, err := r.FinalizeAttempt(ctx, attemptID, "timeout"); err != nil {
			return nil, err
		}
		if a, err = r.loadAttempt(ctx, r.pool, attemptID, false); err != nil {
			return nil, err
		}
	}

	state := &assessmentv1.AttemptState{
		AttemptId:       a.ID,
		AssessmentId:    a.AssessmentID,
		Title:           a.Title,
		Status:          a.Status,
		AllowBacktrack:  a.AllowBacktrack,
		LockForward:     a.LockForward,
		Proctoring:      a.Proctoring,
		ServerNow:       now.Format(time.RFC3339),
		ExpiresAt:       a.ExpiresAt.UTC().Format(time.RFC3339),
		SecondsLeft:     a.secondsLeft(now),
		MaxScore:        a.MaxScore,
		NegativeMarking: a.NegativeMarking,
		ResultsWithheld: resultsWithheld(a.Purpose),
	}

	if state.Sections, err = r.attemptSections(ctx, a.AssessmentID); err != nil {
		return nil, err
	}
	if state.Questions, err = r.attemptQuestions(ctx, attemptID, false); err != nil {
		return nil, err
	}
	return state, nil
}

func (r *Repo) attemptSections(ctx context.Context, assessmentID string) ([]*assessmentv1.AttemptSection, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, title, kind, order_index, COALESCE(duration_minutes, 0)
		FROM   assessment_sections WHERE assessment_id = $1 ORDER BY order_index, title
	`, assessmentID)
	if err != nil {
		return nil, fmt.Errorf("load attempt sections: %w", err)
	}
	defer rows.Close()

	out := []*assessmentv1.AttemptSection{}
	for rows.Next() {
		s := &assessmentv1.AttemptSection{}
		if err := rows.Scan(&s.Id, &s.Title, &s.Kind, &s.OrderIndex, &s.DurationMinutes); err != nil {
			return nil, fmt.Errorf("scan attempt section: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// attemptQuestions projects the frozen paper for the client.
//
// The student projection (includeScores=false) never selects mcq_options.is_correct
// and never returns awarded marks — the answer key does not cross this
// boundary. Recruiter reports and revealed results pass includeScores=true.
func (r *Repo) attemptQuestions(ctx context.Context, attemptID string, includeScores bool) ([]*assessmentv1.AttemptQuestion, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT aq.id::text, aq.section_id::text, aq.kind, aq.order_index, aq.marks,
		       COALESCE(aq.mcq_question_id::text, ''), COALESCE(aq.problem_id::text, ''),
		       COALESCE(m.body, ''), COALESCE(m.kind, ''), COALESCE(p.title, ''),
		       aq.option_order::text[], aq.selected_options::text[],
		       COALESCE(aq.text_answer, ''), COALESCE(aq.submission_id::text, ''),
		       aq.language, aq.code, aq.grading_status, aq.visited, aq.marked_review,
		       aq.time_spent_ms, aq.awarded_marks
		FROM   attempt_questions aq
		LEFT   JOIN mcq_questions m ON m.id = aq.mcq_question_id
		LEFT   JOIN problems      p ON p.id = aq.problem_id
		WHERE  aq.attempt_id = $1
		ORDER  BY aq.order_index
	`, attemptID)
	if err != nil {
		return nil, fmt.Errorf("load attempt questions: %w", err)
	}
	defer rows.Close()

	out := []*assessmentv1.AttemptQuestion{}
	optionOrders := map[string][]string{}
	allOptionIDs := []string{}

	for rows.Next() {
		q := &assessmentv1.AttemptQuestion{}
		var mcqID, optionOrder, selected = "", []string{}, []string{}
		var awarded *float64
		if err := rows.Scan(&q.Id, &q.SectionId, &q.Kind, &q.OrderIndex, &q.Marks,
			&mcqID, &q.ProblemId, &q.Body, &q.McqKind, &q.ProblemTitle,
			&optionOrder, &selected, &q.TextAnswer, &q.SubmissionId,
			&q.Language, &q.Code, &q.GradingStatus, &q.Visited, &q.MarkedReview,
			&q.TimeSpentMs, &awarded); err != nil {
			return nil, fmt.Errorf("scan attempt question: %w", err)
		}
		q.SelectedOptionIds = selected
		if includeScores {
			q.AwardedMarks = awarded
		}
		if mcqID != "" {
			optionOrders[q.Id] = optionOrder
			allOptionIDs = append(allOptionIDs, optionOrder...)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(allOptionIDs) == 0 {
		return out, nil
	}

	correctCol := "false"
	if includeScores {
		correctCol = "is_correct"
	}
	optRows, err := r.pool.Query(ctx, `
		SELECT id::text, body, `+correctCol+`
		FROM   mcq_options WHERE id = ANY($1::uuid[])
	`, allOptionIDs)
	if err != nil {
		return nil, fmt.Errorf("load attempt options: %w", err)
	}
	defer optRows.Close()

	byID := map[string]*assessmentv1.McqOption{}
	for optRows.Next() {
		o := &assessmentv1.McqOption{}
		if err := optRows.Scan(&o.Id, &o.Body, &o.IsCorrect); err != nil {
			return nil, fmt.Errorf("scan attempt option: %w", err)
		}
		byID[o.Id] = o
	}
	if err := optRows.Err(); err != nil {
		return nil, err
	}

	// Rebuild each question's options in the order frozen for this attempt.
	for _, q := range out {
		order := optionOrders[q.Id]
		if len(order) == 0 {
			continue
		}
		q.Options = make([]*assessmentv1.McqOption, 0, len(order))
		for i, id := range order {
			if o, ok := byID[id]; ok {
				q.Options = append(q.Options, &assessmentv1.McqOption{
					Id: o.Id, Body: o.Body, IsCorrect: o.IsCorrect, OrderIndex: int32(i),
				})
			}
		}
	}
	return out, nil
}

// ─── Answering ────────────────────────────────────────────────────────────────

// SaveAnswer records an MCQ selection or a descriptive answer. Every call
// re-checks the deadline, so a client whose clock disagrees with the server
// cannot write past expiry.
func (r *Repo) SaveAnswer(ctx context.Context, req *assessmentv1.SaveAnswerRequest) (int64, error) {
	a, err := r.requireLiveAttempt(ctx, req.AttemptId, req.UserId)
	if err != nil {
		return 0, err
	}

	var kind string
	var orderIndex int32
	err = r.pool.QueryRow(ctx, `
		SELECT kind, order_index FROM attempt_questions WHERE id = $1 AND attempt_id = $2
	`, req.QuestionId, req.AttemptId).Scan(&kind, &orderIndex)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("lookup attempt question: %w", err)
	}

	// On a sequential paper the client only ever shows the candidate a question
	// whose predecessors are done, but the client is not the authority: a
	// hand-rolled request could otherwise write an answer for a question the
	// candidate was never meant to have reached.
	if a.LockForward {
		if err := r.requireEarlierAnswered(ctx, req.AttemptId, orderIndex); err != nil {
			return 0, err
		}
	}

	// A coding question saves a draft, not an answer: the editor's contents are
	// stored verbatim and nothing is graded. Grading still only happens through
	// SubmitAttemptCode, so this cannot be used to sneak an ungraded run past
	// the judge — but it does mean a candidate who navigates away, refreshes or
	// crashes keeps the code they had written.
	if kind == "coding" {
		if _, err := r.pool.Exec(ctx, `
			UPDATE attempt_questions SET
				language      = COALESCE(NULLIF($3, ''), language),
				code          = $4,
				marked_review = $5,
				visited       = true,
				time_spent_ms = time_spent_ms + GREATEST($6, 0)
			WHERE id = $1 AND attempt_id = $2
		`, req.QuestionId, req.AttemptId, req.Language, req.Code,
			req.MarkedReview, req.TimeSpentMs); err != nil {
			return 0, fmt.Errorf("save coding draft: %w", err)
		}
		return a.secondsLeft(time.Now().UTC()), nil
	}

	if !a.AllowBacktrack {
		var furthest int32
		if err := r.pool.QueryRow(ctx, `
			SELECT COALESCE(MAX(order_index), -1) FROM attempt_questions
			WHERE attempt_id = $1 AND visited = true
		`, req.AttemptId).Scan(&furthest); err != nil {
			return 0, fmt.Errorf("check backtrack: %w", err)
		}
		if orderIndex < furthest {
			return 0, fmt.Errorf("this test does not allow going back to an earlier question")
		}
	}

	selected := req.SelectedOptionIds
	if req.ClearAnswer {
		selected = []string{}
	}
	if selected == nil {
		selected = []string{}
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE attempt_questions SET
			selected_options = $3::uuid[],
			text_answer      = CASE WHEN $4 THEN NULL ELSE $5 END,
			marked_review    = $6,
			visited          = true,
			time_spent_ms    = time_spent_ms + GREATEST($7, 0)
		WHERE id = $1 AND attempt_id = $2
	`, req.QuestionId, req.AttemptId, selected, req.ClearAnswer, req.TextAnswer,
		req.MarkedReview, req.TimeSpentMs)
	if err != nil {
		return 0, fmt.Errorf("save answer: %w", err)
	}
	return a.secondsLeft(time.Now().UTC()), nil
}

// requireEarlierAnswered enforces a lock_forward paper's rule: every question
// before orderIndex must already be answered.
//
// "Answered" matches what the player's palette shows as answered — a committed
// selection or text for an MCQ or descriptive question, a judged submission for
// a coding one. A saved coding draft deliberately does not count: writing code
// into the editor is not solving the question.
func (r *Repo) requireEarlierAnswered(ctx context.Context, attemptID string, orderIndex int32) error {
	var pending int32
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MIN(order_index), -1) FROM attempt_questions
		WHERE  attempt_id = $1 AND order_index < $2
		  AND  CASE WHEN kind = 'coding' THEN submission_id IS NULL
		            ELSE cardinality(selected_options) = 0
		                 AND COALESCE(btrim(text_answer), '') = ''
		       END
	`, attemptID, orderIndex).Scan(&pending); err != nil {
		return fmt.Errorf("check sequential lock: %w", err)
	}
	if pending >= 0 {
		return fmt.Errorf("answer question %d before moving on", pending+1)
	}
	return nil
}

// requireLiveAttempt is the single gate every write path goes through: right
// owner, right status, deadline not passed. An attempt found past its deadline
// is finalized here rather than left to drift.
func (r *Repo) requireLiveAttempt(ctx context.Context, attemptID, userID string) (*attemptCtx, error) {
	a, err := r.loadAttempt(ctx, r.pool, attemptID, false)
	if err != nil {
		return nil, err
	}
	if a.UserID != userID {
		return nil, ErrForbidden
	}
	if a.Status != "in_progress" {
		return nil, ErrAttemptClosed
	}
	if time.Now().UTC().After(a.ExpiresAt) {
		if _, err := r.FinalizeAttempt(ctx, attemptID, "timeout"); err != nil {
			return nil, err
		}
		return nil, ErrAttemptClosed
	}
	return a, nil
}

// AttemptCodingQuestion validates that a coding question belongs to this live
// attempt and returns the problem id to grade against.
func (r *Repo) AttemptCodingQuestion(ctx context.Context, attemptID, userID, questionID string) (problemID string, err error) {
	a, err := r.requireLiveAttempt(ctx, attemptID, userID)
	if err != nil {
		return "", err
	}
	var orderIndex int32
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(problem_id::text, ''), order_index FROM attempt_questions
		WHERE id = $1 AND attempt_id = $2 AND kind = 'coding'
	`, questionID, attemptID).Scan(&problemID, &orderIndex)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lookup coding question: %w", err)
	}
	// Running the judge is how a coding question gets answered, so it is also
	// where a sequential paper has to be enforced.
	if a.LockForward {
		if err := r.requireEarlierAnswered(ctx, attemptID, orderIndex); err != nil {
			return "", err
		}
	}
	if problemID == "" {
		return "", fmt.Errorf("this question has no problem attached")
	}
	return problemID, nil
}

// RecordCodeSubmission links a submission-service submission to the attempt
// question so the graded event can be routed back when the judge finishes.
func (r *Repo) RecordCodeSubmission(ctx context.Context, attemptID, questionID, submissionID, language, code string) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin record submission: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `
		INSERT INTO attempt_submissions (attempt_question_id, submission_id, language, code)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (submission_id) DO NOTHING
	`, questionID, submissionID, language, code); err != nil {
		return 0, fmt.Errorf("insert attempt submission: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE attempt_questions
		SET    submission_id = $3, language = $4, code = $5,
		       grading_status = 'pending', visited = true
		WHERE  id = $1 AND attempt_id = $2
	`, questionID, attemptID, submissionID, language, code); err != nil {
		return 0, fmt.Errorf("mark question pending: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit record submission: %w", err)
	}

	a, err := r.loadAttempt(ctx, r.pool, attemptID, false)
	if err != nil {
		return 0, err
	}
	return a.secondsLeft(time.Now().UTC()), nil
}

// GetAttemptSubmissionStatus reports the judge verdict for one submission
// without exposing hidden test-case content — only counts and status.
func (r *Repo) GetAttemptSubmissionStatus(ctx context.Context, attemptID, userID, submissionID string) (*assessmentv1.GetAttemptSubmissionResponse, error) {
	a, err := r.loadAttempt(ctx, r.pool, attemptID, false)
	if err != nil {
		return nil, err
	}
	if a.UserID != userID {
		return nil, ErrForbidden
	}

	out := &assessmentv1.GetAttemptSubmissionResponse{SubmissionId: submissionID}
	var passed, total *int32
	err = r.pool.QueryRow(ctx, `
		SELECT s.status, s.passed_count, s.total_count
		FROM   attempt_submissions s
		JOIN   attempt_questions q ON q.id = s.attempt_question_id
		WHERE  s.submission_id = $1 AND q.attempt_id = $2
	`, submissionID, attemptID).Scan(&out.Status, &passed, &total)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get submission status: %w", err)
	}
	if passed != nil {
		out.PassedCount = *passed
	}
	if total != nil {
		out.TotalCount = *total
	}
	return out, nil
}

// ─── Proctoring ───────────────────────────────────────────────────────────────

// RecordProctorEvent appends to the integrity ledger and decays the attempt's
// integrity score.
//
// Only an explicit configured tab-switch limit terminates an attempt. Every
// other signal is advisory evidence for the recruiter — client-side proctoring
// false-positives often enough that silently failing a candidate on it would be
// worse than the cheating it catches.
func (r *Repo) RecordProctorEvent(ctx context.Context, req *assessmentv1.RecordProctorEventRequest) (*assessmentv1.RecordProctorEventResponse, error) {
	a, err := r.loadAttempt(ctx, r.pool, req.AttemptId, false)
	if err != nil {
		return nil, err
	}
	if a.UserID != req.UserId {
		return nil, ErrForbidden
	}
	if a.Status != "in_progress" {
		// Late events are still worth keeping, but nothing more happens.
		return &assessmentv1.RecordProctorEventResponse{IntegrityScore: a.IntegrityScore}, nil
	}

	penalty := grading.IntegrityPenalty(req.Kind)
	var score float64
	err = r.pool.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO proctor_events (attempt_id, kind, detail) VALUES ($1, $2, $3)
		)
		UPDATE attempts SET integrity_score = GREATEST(integrity_score - $4, 0)
		WHERE  id = $1
		RETURNING integrity_score
	`, req.AttemptId, req.Kind, req.Detail, penalty).Scan(&score)
	if err != nil {
		return nil, fmt.Errorf("record proctor event: %w", err)
	}

	resp := &assessmentv1.RecordProctorEventResponse{IntegrityScore: score}

	limit := a.Proctoring.TabSwitchLimit
	if limit > 0 && (req.Kind == "tab_blur" || req.Kind == "fullscreen_exit") {
		var breaches int32
		if err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM proctor_events
			WHERE attempt_id = $1 AND kind IN ('tab_blur', 'fullscreen_exit')
		`, req.AttemptId).Scan(&breaches); err != nil {
			return nil, fmt.Errorf("count proctor breaches: %w", err)
		}
		if breaches >= limit {
			if _, err := r.FinalizeAttempt(ctx, req.AttemptId, "proctor"); err != nil {
				return nil, err
			}
			resp.Terminated = true
			resp.Warning = fmt.Sprintf("Test ended: you left the test window %d times.", breaches)
			return resp, nil
		}
		if remaining := limit - breaches; remaining <= 2 {
			resp.Warning = fmt.Sprintf("Warning: %d more switches will end your test.", remaining)
		}
	}
	return resp, nil
}
