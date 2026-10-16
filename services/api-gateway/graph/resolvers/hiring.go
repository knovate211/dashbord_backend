package resolvers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	pkgauth "github.com/knovate211/pkg/auth"
)

// HiringHandler runs the candidate side of a company's hiring test: the
// hiring team adds candidates, each candidate is emailed a personal link, and
// that link signs them straight into the test — the same hand-off the
// scholarship funnel uses, but driven by a recruiter instead of a public form.
//
// Candidates are kept in their own table, hiring_candidates, and get the user
// role 'candidate'. They are not students: the Users screen and dashboard
// counts leave them out, and the student app confines their session to the
// test (see App.tsx).
//
// Security rules, all load-bearing:
//   - The link is a login credential, so it is only ever emailed. The recruiter
//     never sees it: a recruiter who could copy a candidate's link could add
//     anyone's email and sign in as them.
//   - A link never opens a staff account. Adding an admin's or recruiter's
//     address is refused, and the claim re-checks the role.
//   - Only the SHA-256 digest of the link token is stored.
//   - Eligibility is the assessment_invites row, which assessment-service
//     enforces when the test starts; this handler writes that row and grants
//     access no other way.
type HiringHandler struct {
	Pool      *pgxpool.Pool
	Log       *zap.Logger
	jwtSecret string
	appBase   string
	limiter   *rateLimiter
	// The scholarship mailer's delivery is generic (recipient, subject, text,
	// HTML) and already carries the SMTP configuration, so it is reused.
	mailer *scholarshipMailer
}

// hiringLinkTTL is how long a candidate's link works unless the recruiter
// picks a date. A week covers "invited on Monday, sits it at the weekend".
const hiringLinkTTL = 7 * 24 * time.Hour

// maxCandidatesPerRequest bounds one "add candidates" call.
const maxCandidatesPerRequest = 500

// NewHiringHandler wires the handler and ensures its table exists.
func NewHiringHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger, jwtSecret, appBase string) (*HiringHandler, error) {
	h := &HiringHandler{
		Pool:      pool,
		Log:       log,
		jwtSecret: jwtSecret,
		appBase:   strings.TrimRight(appBase, "/"),
		// Claims only, and off (0) by default like the other per-IP limits: a
		// campus drive has a lab of candidates opening links from one address,
		// and a 32-byte token cannot be guessed anyway. Set it to stop a flood.
		limiter: newRateLimiter(envInt("HIRING_CLAIM_IP_LIMIT", 0), envMinutes("HIRING_CLAIM_IP_WINDOW_MIN", 10)),
		mailer:  newScholarshipMailer(log),
	}
	if err := h.ensureTable(ctx); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *HiringHandler) ensureTable(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS hiring_candidates (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			assessment_id    UUID NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			company_id       UUID,
			name             TEXT NOT NULL,
			email            TEXT NOT NULL,
			phone            TEXT NOT NULL DEFAULT '',
			user_id          UUID,
			invite_id        UUID,
			-- Only the digest; the raw token exists in the email and nowhere else.
			claim_token_hash TEXT UNIQUE,
			claim_expires_at TIMESTAMPTZ,
			claimed_at       TIMESTAMPTZ,
			emailed_at       TIMESTAMPTZ,
			email_error      TEXT NOT NULL DEFAULT '',
			added_by         UUID,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		-- One row per person per test; adding someone again refreshes their link.
		CREATE UNIQUE INDEX IF NOT EXISTS uq_hiring_candidate_email
			ON hiring_candidates (assessment_id, lower(email));
		CREATE INDEX IF NOT EXISTS idx_hiring_candidates_company ON hiring_candidates (company_id);
	`)
	if err != nil {
		return fmt.Errorf("create hiring_candidates: %w", err)
	}
	return nil
}

// ─── Routing ──────────────────────────────────────────────────────────────────

// ServeHTTP handles /api/hiring/*. Only /claim is public.
//
//	POST   /api/hiring/claim                          public — link → session
//	GET    /api/hiring/assessments/{id}/candidates    recruiter/admin
//	POST   /api/hiring/assessments/{id}/candidates    recruiter/admin
//	POST   /api/hiring/candidates/{id}/resend         recruiter/admin
//	DELETE /api/hiring/candidates/{id}                recruiter/admin
func (h *HiringHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/hiring"), "/")
	seg := strings.Split(path, "/")

	if path == "claim" && r.Method == http.MethodPost {
		h.handleClaim(w, r)
		return
	}

	role := middleware.RoleFromContext(r.Context())
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		h.fail(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if role != "admin" && role != "recruiter" {
		h.fail(w, http.StatusForbidden, "recruiter access required")
		return
	}

	switch {
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "candidates" && r.Method == http.MethodGet:
		h.listCandidates(w, r, role, userID, seg[1])
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "candidates" && r.Method == http.MethodPost:
		h.addCandidates(w, r, role, userID, seg[1])
	case len(seg) == 3 && seg[0] == "candidates" && seg[2] == "resend" && r.Method == http.MethodPost:
		h.resend(w, r, role, userID, seg[1])
	case len(seg) == 2 && seg[0] == "candidates" && r.Method == http.MethodDelete:
		h.remove(w, r, role, userID, seg[1])
	default:
		h.fail(w, http.StatusNotFound, "unknown hiring endpoint")
	}
}

// ─── Authorization ────────────────────────────────────────────────────────────

type hiringTest struct {
	ID, CompanyID, CompanyName, Title, Status string
	Duration, TotalMarks                      int32
}

// loadTest returns the hiring test if the caller may manage it. Admins may
// manage any; a recruiter must be a member of the owning company. It writes
// the error response itself and returns nil when access is refused.
func (h *HiringHandler) loadTest(w http.ResponseWriter, ctx context.Context, role, userID, assessmentID string) *hiringTest {
	t := &hiringTest{ID: assessmentID}
	var purpose string
	var allowed bool
	err := h.Pool.QueryRow(ctx, `
		SELECT COALESCE(a.company_id::text, ''), COALESCE(c.name, ''), a.title, a.status, a.purpose,
		       a.duration_minutes, a.total_marks,
		       ($2 = 'admin' OR EXISTS (
		           SELECT 1 FROM company_members m
		           WHERE  m.company_id = a.company_id AND m.user_id::text = $3))
		FROM   assessments a
		LEFT   JOIN companies c ON c.id = a.company_id
		WHERE  a.id::text = $1
	`, assessmentID, role, userID).Scan(&t.CompanyID, &t.CompanyName, &t.Title, &t.Status, &purpose,
		&t.Duration, &t.TotalMarks, &allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "test not found")
		return nil
	}
	if err != nil {
		h.Log.Error("load hiring test failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load the test")
		return nil
	}
	if !allowed {
		h.fail(w, http.StatusForbidden, "not a member of this company")
		return nil
	}
	if purpose != "hiring" {
		h.fail(w, http.StatusBadRequest, "candidates can only be added to hiring tests")
		return nil
	}
	return t
}

// candidateTest resolves a candidate row to its test, with the same access
// check, so a candidate id alone never reaches another company's data.
func (h *HiringHandler) candidateTest(w http.ResponseWriter, ctx context.Context, role, userID, candidateID string) (*hiringTest, *candidateRow) {
	c := &candidateRow{ID: candidateID}
	var assessmentID string
	err := h.Pool.QueryRow(ctx, `
		SELECT assessment_id::text, name, email, COALESCE(user_id::text, ''), COALESCE(invite_id::text, '')
		FROM   hiring_candidates WHERE id::text = $1
	`, candidateID).Scan(&assessmentID, &c.Name, &c.Email, &c.UserID, &c.InviteID)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "candidate not found")
		return nil, nil
	}
	if err != nil {
		h.Log.Error("load hiring candidate failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load the candidate")
		return nil, nil
	}
	t := h.loadTest(w, ctx, role, userID, assessmentID)
	if t == nil {
		return nil, nil
	}
	return t, c
}

type candidateRow struct {
	ID, Name, Email, UserID, InviteID string
}

// ─── List ─────────────────────────────────────────────────────────────────────

type candidateView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Phone       string   `json:"phone"`
	Status      string   `json:"status"` // invited | opened | in_progress | submitted | … | expired
	AddedAt     string   `json:"added_at"`
	EmailedAt   string   `json:"emailed_at,omitempty"`
	EmailError  string   `json:"email_error,omitempty"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
	OpenedAt    string   `json:"opened_at,omitempty"`
	SubmittedAt string   `json:"submitted_at,omitempty"`
	AttemptID   string   `json:"attempt_id,omitempty"`
	Score       *float64 `json:"score,omitempty"`
	MaxScore    *float64 `json:"max_score,omitempty"`
}

func (h *HiringHandler) listCandidates(w http.ResponseWriter, r *http.Request, role, userID, assessmentID string) {
	ctx := r.Context()
	if h.loadTest(w, ctx, role, userID, assessmentID) == nil {
		return
	}
	rows, err := h.Pool.Query(ctx, `
		SELECT h.id::text, h.name, h.email, h.phone, h.created_at, h.emailed_at, h.email_error,
		       h.claim_expires_at, h.claimed_at,
		       COALESCE(at.id::text, ''), COALESCE(at.status, ''), at.score, at.max_score, at.submitted_at
		FROM   hiring_candidates h
		LEFT   JOIN LATERAL (
		         SELECT id, status, score::float8 AS score, max_score::float8 AS max_score, submitted_at
		         FROM   attempts
		         WHERE  assessment_id = h.assessment_id AND user_id = h.user_id
		         ORDER  BY started_at DESC LIMIT 1
		       ) at ON true
		WHERE  h.assessment_id::text = $1
		ORDER  BY h.created_at DESC
	`, assessmentID)
	if err != nil {
		h.Log.Error("list hiring candidates failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load candidates")
		return
	}
	defer rows.Close()

	now := time.Now().UTC()
	out := []candidateView{}
	for rows.Next() {
		var v candidateView
		var added time.Time
		var emailed, expires, claimed, submitted *time.Time
		var attemptStatus string
		if err := rows.Scan(&v.ID, &v.Name, &v.Email, &v.Phone, &added, &emailed, &v.EmailError,
			&expires, &claimed, &v.AttemptID, &attemptStatus, &v.Score, &v.MaxScore, &submitted); err != nil {
			h.Log.Error("scan hiring candidate failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load candidates")
			return
		}
		v.AddedAt = added.Format(time.RFC3339)
		v.EmailedAt, v.ExpiresAt, v.OpenedAt, v.SubmittedAt = fmtTS(emailed), fmtTS(expires), fmtTS(claimed), fmtTS(submitted)
		// The attempt is the truth once there is one; before that, the link.
		switch {
		case attemptStatus != "":
			v.Status = attemptStatus
		case expires != nil && now.After(*expires):
			v.Status = "expired"
		case claimed != nil:
			v.Status = "opened"
		default:
			v.Status = "invited"
		}
		if attemptStatus == "" || attemptStatus == "in_progress" {
			v.Score, v.MaxScore = nil, nil
		}
		out = append(out, v)
	}
	h.json(w, http.StatusOK, map[string]any{"candidates": out})
}

func fmtTS(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ─── Add ──────────────────────────────────────────────────────────────────────

type candidateInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type addResult struct {
	Email   string `json:"email"`
	Status  string `json:"status"` // added | updated | skipped
	Message string `json:"message,omitempty"`
}

func (h *HiringHandler) addCandidates(w http.ResponseWriter, r *http.Request, role, userID, assessmentID string) {
	ctx := r.Context()
	t := h.loadTest(w, ctx, role, userID, assessmentID)
	if t == nil {
		return
	}
	if t.Status != "published" {
		h.fail(w, http.StatusBadRequest, "publish the test before adding candidates — the email takes them straight into it")
		return
	}

	var body struct {
		Candidates []candidateInput `json:"candidates"`
		ExpiresAt  string           `json:"expires_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Candidates) == 0 {
		h.fail(w, http.StatusBadRequest, "add at least one candidate")
		return
	}
	if len(body.Candidates) > maxCandidatesPerRequest {
		h.fail(w, http.StatusBadRequest, fmt.Sprintf("add at most %d candidates at a time", maxCandidatesPerRequest))
		return
	}
	expires, err := linkExpiry(body.ExpiresAt)
	if err != nil {
		h.fail(w, http.StatusBadRequest, err.Error())
		return
	}

	results := make([]addResult, 0, len(body.Candidates))
	seen := map[string]bool{}
	for _, in := range body.Candidates {
		email := strings.ToLower(clip(in.Email, 200))
		name := clip(in.Name, 120)
		if !looksLikeEmail(email) {
			results = append(results, addResult{Email: in.Email, Status: "skipped", Message: "not a valid email address"})
			continue
		}
		if seen[email] {
			results = append(results, addResult{Email: email, Status: "skipped", Message: "listed twice"})
			continue
		}
		seen[email] = true
		if name == "" {
			name = strings.Split(email, "@")[0]
		}

		res, raw, err := h.provision(ctx, t, userID, name, email, clip(in.Phone, 40), expires)
		if err != nil {
			results = append(results, addResult{Email: email, Status: "skipped", Message: err.Error()})
			continue
		}
		results = append(results, res.addResult)
		h.sendInvite(res.candidateID, name, email, t, raw, expires)
	}

	added := 0
	for _, r := range results {
		if r.Status != "skipped" {
			added++
		}
	}
	h.json(w, http.StatusOK, map[string]any{
		"results": results, "added": added, "email_enabled": h.mailer.enabled(),
	})
}

// provisionResult carries the new row's id alongside the public result.
type provisionResult = struct {
	addResult
	candidateID string
}

// provision creates (or refreshes) one candidate: account, invite and a fresh
// link token, in one transaction. It returns the raw token for the email.
func (h *HiringHandler) provision(ctx context.Context, t *hiringTest, actorID, name, email, phone string, expires time.Time) (provisionResult, string, error) {
	var out provisionResult
	out.Email = email

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return out, "", errors.New("could not add, please try again")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	// A link must never open a staff account, so staff addresses are refused
	// outright rather than quietly given a candidate link.
	var existingRole string
	err = tx.QueryRow(ctx, `SELECT role FROM users WHERE lower(email) = $1`, email).Scan(&existingRole)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("look up candidate account failed", zap.Error(err))
		return out, "", errors.New("could not add, please try again")
	}
	if existingRole == "admin" || existingRole == "recruiter" {
		return out, "", errors.New("this email belongs to a staff account")
	}

	// Someone who has already sat this test is not re-invited.
	var spent bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM attempts at JOIN users u ON u.id = at.user_id
		  WHERE  at.assessment_id::text = $1 AND lower(u.email) = $2 AND at.status <> 'in_progress')
	`, t.ID, email).Scan(&spent); err != nil {
		h.Log.Error("check candidate attempts failed", zap.Error(err))
		return out, "", errors.New("could not add, please try again")
	}
	if spent {
		return out, "", errors.New("already took this test")
	}

	// The account the attempt is keyed to. A new one is a 'candidate' with no
	// usable password — the emailed link is the way in. An existing student
	// keeps their role, name and password untouched: a recruiter must not be
	// able to change somebody's account by typing their email here.
	placeholder, err := pkgauth.UnusablePassword()
	if err != nil {
		return out, "", errors.New("could not add, please try again")
	}
	var candidateUserID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, name, password, role)
		VALUES ($1, $2, $3, 'candidate')
		ON CONFLICT (email) DO UPDATE SET updated_at = users.updated_at
		RETURNING id::text
	`, email, name, placeholder).Scan(&candidateUserID); err != nil {
		h.Log.Error("upsert candidate account failed", zap.Error(err))
		return out, "", errors.New("could not add, please try again")
	}

	// Eligibility: the invite row assessment-service checks at start.
	inviteToken, err := randomToken(24)
	if err != nil {
		return out, "", errors.New("could not add, please try again")
	}
	var inviteID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO assessment_invites (assessment_id, email, user_id, token, status, expires_at, sent_at)
		VALUES ($1::uuid, $2, $3::uuid, $4, 'invited', $5, now())
		ON CONFLICT (assessment_id, email) DO UPDATE
			SET user_id = EXCLUDED.user_id, expires_at = EXCLUDED.expires_at, sent_at = now()
		RETURNING id::text
	`, t.ID, email, candidateUserID, inviteToken, expires).Scan(&inviteID); err != nil {
		h.Log.Error("upsert candidate invite failed", zap.Error(err))
		return out, "", errors.New("could not add, please try again")
	}

	raw, err := randomToken(32)
	if err != nil {
		return out, "", errors.New("could not add, please try again")
	}

	// Insert, or refresh an existing row with a new link (the old one stops
	// working, since only the latest digest is kept).
	var isNew bool
	if err := tx.QueryRow(ctx, `
		WITH upd AS (
		  UPDATE hiring_candidates
		  SET    name = $3, phone = CASE WHEN $4 = '' THEN phone ELSE $4 END,
		         user_id = $5::uuid, invite_id = $6::uuid,
		         claim_token_hash = $7, claim_expires_at = $8, email_error = '', updated_at = now()
		  WHERE  assessment_id = $1::uuid AND lower(email) = $2
		  RETURNING id
		), ins AS (
		  INSERT INTO hiring_candidates (assessment_id, company_id, name, email, phone, user_id, invite_id,
		                                 claim_token_hash, claim_expires_at, added_by)
		  SELECT $1::uuid, NULLIF($9, '')::uuid, $3, $2, $4, $5::uuid, $6::uuid, $7, $8, NULLIF($10, '')::uuid
		  WHERE  NOT EXISTS (SELECT 1 FROM upd)
		  RETURNING id
		)
		SELECT id::text, false FROM upd UNION ALL SELECT id::text, true FROM ins
	`, t.ID, email, name, phone, candidateUserID, inviteID, sha256Hex(raw), expires,
		t.CompanyID, actorID).Scan(&out.candidateID, &isNew); err != nil {
		h.Log.Error("upsert hiring candidate failed", zap.Error(err))
		return out, "", errors.New("could not add, please try again")
	}

	if err := tx.Commit(ctx); err != nil {
		h.Log.Error("commit hiring candidate failed", zap.Error(err))
		return out, "", errors.New("could not add, please try again")
	}
	out.Status = "added"
	if !isNew {
		out.Status, out.Message = "updated", "already on the list — a new link was sent"
	}
	return out, raw, nil
}

// linkExpiry parses the recruiter's chosen expiry, or defaults to a week.
func linkExpiry(s string) (time.Time, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(s) == "" {
		return now.Add(hiringLinkTTL), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, errors.New("expires_at must be an RFC 3339 date-time")
	}
	if !t.After(now) {
		return time.Time{}, errors.New("the link expiry must be in the future")
	}
	if t.After(now.Add(90 * 24 * time.Hour)) {
		return time.Time{}, errors.New("links can last at most 90 days")
	}
	return t.UTC(), nil
}

// ─── Resend ───────────────────────────────────────────────────────────────────

// resend rotates the candidate's link and emails it again. The old link stops
// working. Used when an email bounced, went to spam, or the link expired.
func (h *HiringHandler) resend(w http.ResponseWriter, r *http.Request, role, userID, candidateID string) {
	ctx := r.Context()
	t, c := h.candidateTest(w, ctx, role, userID, candidateID)
	if t == nil {
		return
	}
	if t.Status != "published" {
		h.fail(w, http.StatusBadRequest, "the test is not published")
		return
	}
	res, raw, err := h.provision(ctx, t, userID, c.Name, c.Email, "", time.Now().UTC().Add(hiringLinkTTL))
	if err != nil {
		h.fail(w, http.StatusConflict, err.Error())
		return
	}
	h.sendInvite(res.candidateID, c.Name, c.Email, t, raw, time.Now().UTC().Add(hiringLinkTTL))
	h.json(w, http.StatusOK, map[string]any{"resent": true, "email_enabled": h.mailer.enabled()})
}

// ─── Remove ───────────────────────────────────────────────────────────────────

// remove withdraws an invitation that has not been used. Once a candidate has
// started, their attempt is part of the results and stays.
func (h *HiringHandler) remove(w http.ResponseWriter, r *http.Request, role, userID, candidateID string) {
	ctx := r.Context()
	t, c := h.candidateTest(w, ctx, role, userID, candidateID)
	if t == nil {
		return
	}
	var started bool
	if err := h.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM attempts WHERE assessment_id::text = $1 AND user_id::text = $2)
	`, t.ID, c.UserID).Scan(&started); err != nil {
		h.fail(w, http.StatusInternalServerError, "could not remove the candidate")
		return
	}
	if started {
		h.fail(w, http.StatusConflict, "this candidate has already started the test, so their result is kept")
		return
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not remove the candidate")
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `DELETE FROM hiring_candidates WHERE id::text = $1`, candidateID); err != nil {
		h.fail(w, http.StatusInternalServerError, "could not remove the candidate")
		return
	}
	if c.InviteID != "" {
		if _, err := tx.Exec(ctx, `DELETE FROM assessment_invites WHERE id::text = $1`, c.InviteID); err != nil {
			h.fail(w, http.StatusInternalServerError, "could not remove the candidate")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		h.fail(w, http.StatusInternalServerError, "could not remove the candidate")
		return
	}

	// Tidy the account away when it was only ever a candidate for this test.
	// Best effort: a leftover account with no password and no invite is inert.
	if _, err := h.Pool.Exec(ctx, `
		DELETE FROM users u
		WHERE  u.id::text = $1 AND u.role = 'candidate'
		  AND  NOT EXISTS (SELECT 1 FROM hiring_candidates h WHERE h.user_id = u.id)
		  AND  NOT EXISTS (SELECT 1 FROM attempts a WHERE a.user_id = u.id)
	`, c.UserID); err != nil {
		h.Log.Warn("remove unused candidate account failed", zap.Error(err))
	}
	h.json(w, http.StatusOK, map[string]any{"removed": true})
}

// ─── Email ────────────────────────────────────────────────────────────────────

// sendInvite emails the link in the background and records the outcome on the
// candidate row, so the recruiter's list shows "sent" or why it failed. The
// request does not wait on SMTP: a list of 200 candidates would time out.
func (h *HiringHandler) sendInvite(candidateID, name, email string, t *hiringTest, raw string, expires time.Time) {
	link := h.appBase + "/hiring/start?t=" + raw
	company := t.CompanyName
	if company == "" {
		company = "Knovate"
	}
	subject := fmt.Sprintf("%s has invited you to take a test: %s", company, t.Title)
	text := strings.Join([]string{
		"Hi " + name + ",",
		"",
		company + " has invited you to take an online assessment as part of their hiring process.",
		"",
		"Test:      " + t.Title,
		fmt.Sprintf("Duration:  %d minutes", t.Duration),
		"Link valid until: " + expires.Format("2 Jan 2006, 15:04 MST"),
		"",
		"Start your test here — the link is personal to you, so please do not share it:",
		link,
		"",
		"Before you begin: use a laptop or desktop with a stable internet connection and set",
		"aside the full duration in one sitting. The timer runs on our servers once you start,",
		"so closing the tab does not stop it.",
		"",
		"Good luck.",
	}, "\n")
	html := hiringInviteHTML(name, company, t.Title, link, t.Duration, t.TotalMarks, expires)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.Log.Error("hiring invite email panicked", zap.Any("panic", r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if !h.mailer.enabled() {
			// Local development: the link has to reach somebody, and that is
			// whoever reads the logs. Recorded as an error so the recruiter
			// sees nothing was actually sent.
			h.Log.Info("hiring test link (email disabled)", zap.String("email", email), zap.String("url", link))
			_, _ = h.Pool.Exec(ctx, `UPDATE hiring_candidates SET email_error = $2, updated_at = now() WHERE id::text = $1`,
				candidateID, "email is not configured on the server")
			return
		}
		if err := h.mailer.deliver([]string{email}, "", subject, text, html, email); err != nil {
			_, _ = h.Pool.Exec(ctx, `UPDATE hiring_candidates SET email_error = $2, updated_at = now() WHERE id::text = $1`,
				candidateID, clip(err.Error(), 300))
			return
		}
		_, _ = h.Pool.Exec(ctx, `UPDATE hiring_candidates SET emailed_at = now(), email_error = '', updated_at = now() WHERE id::text = $1`,
			candidateID)
	}()
}

// ─── Claim (public) ───────────────────────────────────────────────────────────

// handleClaim exchanges an emailed link for a session. The link keeps working
// until it expires or the test is handed in, so a candidate who refreshes the
// page, or opens it on another device, is not locked out.
func (h *HiringHandler) handleClaim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	raw := clip(body.Token, 200)
	if raw == "" {
		h.fail(w, http.StatusBadRequest, "this link is missing its access code")
		return
	}
	if !h.limiter.allow("claim:" + clientIP(r)) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts from this network — please try again in a few minutes")
		return
	}

	ctx := r.Context()
	var candidateID, userID, assessmentID, email, userName, role, title, company, inviteToken, assessmentStatus string
	var expiresAt *time.Time
	err := h.Pool.QueryRow(ctx, `
		SELECT h.id::text, h.user_id::text, h.assessment_id::text, u.email, u.name, u.role,
		       a.title, a.status, COALESCE(c.name, ''), COALESCE(i.token, ''), h.claim_expires_at
		FROM   hiring_candidates h
		JOIN   users u        ON u.id = h.user_id
		JOIN   assessments a  ON a.id = h.assessment_id
		LEFT   JOIN companies c          ON c.id = a.company_id
		LEFT   JOIN assessment_invites i ON i.id = h.invite_id
		WHERE  h.claim_token_hash = $1
	`, sha256Hex(raw)).Scan(&candidateID, &userID, &assessmentID, &email, &userName, &role,
		&title, &assessmentStatus, &company, &inviteToken, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Same answer for a token that never existed and one that was replaced.
		h.fail(w, http.StatusUnauthorized, "this link is no longer valid — ask the hiring team to send you a new one")
		return
	}
	if err != nil {
		h.Log.Error("look up hiring claim failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not open your test, please try again")
		return
	}
	if expiresAt != nil && time.Now().UTC().After(*expiresAt) {
		h.fail(w, http.StatusUnauthorized, "this link has expired — ask the hiring team to send you a new one")
		return
	}
	if assessmentStatus != "published" {
		h.fail(w, http.StatusConflict, "this test is not open at the moment — please contact the hiring team")
		return
	}
	// Re-checked here, not only when the candidate was added: an account can
	// have been promoted since, and a link must never open a staff session.
	if role == "admin" || role == "recruiter" {
		h.Log.Warn("hiring claim refused for staff account", zap.String("email", email))
		h.fail(w, http.StatusForbidden, "this link cannot be used — please contact the hiring team")
		return
	}
	var done bool
	if err := h.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM attempts
		               WHERE assessment_id::text = $1 AND user_id::text = $2 AND status <> 'in_progress')
	`, assessmentID, userID).Scan(&done); err == nil && done {
		h.fail(w, http.StatusConflict, "you have already completed this test")
		return
	}

	if _, err := h.Pool.Exec(ctx, `
		UPDATE hiring_candidates SET claimed_at = COALESCE(claimed_at, now()), updated_at = now()
		WHERE  id::text = $1
	`, candidateID); err != nil {
		h.Log.Warn("mark hiring claim failed", zap.Error(err))
	}

	token, err := pkgauth.GenerateToken(userID, email, role, h.jwtSecret, 24*time.Hour)
	if err != nil {
		h.Log.Error("mint candidate session failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not open your test, please try again")
		return
	}
	h.json(w, http.StatusOK, map[string]any{
		"token":        token,
		"user":         map[string]any{"id": userID, "email": email, "name": userName, "role": role},
		"assessmentId": assessmentID,
		"inviteToken":  inviteToken,
		"title":        title,
		"companyName":  company,
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (h *HiringHandler) json(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *HiringHandler) fail(w http.ResponseWriter, code int, msg string) {
	h.json(w, code, map[string]string{"error": msg})
}
