package resolvers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// slugRe is what a certification exam's URL segment may contain.
var slugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// Admin surface for certification exams, mounted under /api/admin by
// AdminHandler so it inherits the role guard there.
//
//	GET    /api/admin/certification-exams        — exams on offer
//	POST   /api/admin/certification-exams        — create or update one
//	DELETE /api/admin/certification-exams/{id}   — remove one that has no registrations
//	GET    /api/admin/certifications             — registrations, with live attempt state
//	GET    /api/admin/certifications/export.csv  — the same rows as a spreadsheet
//	PATCH  /api/admin/certifications/{id}        — notes, refund, issue or revoke
//	POST   /api/admin/certifications/{id}/resend — a fresh link, emailed again
//	POST   /api/admin/certifications/{id}/extend — push the expiry out
//
// Like the scholarship screens, the attempt is read live rather than copied
// onto the registration: coding answers grade asynchronously, so a cached score
// would be stale exactly when staff are watching.

type certExamAdmin struct {
	ID              string  `json:"id"`
	Slug            string  `json:"slug"`
	Title           string  `json:"title"`
	CourseID        string  `json:"course_id"`
	AssessmentID    string  `json:"assessment_id"`
	AssessmentTitle string  `json:"assessment_title"`
	AssessmentState string  `json:"assessment_status"`
	PriceRupees     int64   `json:"price_rupees"`
	PassPercent     float64 `json:"pass_percent"`
	LinkValidDays   int     `json:"link_valid_days"`
	ResitWaitDays   int     `json:"resit_wait_days"`
	Summary         string  `json:"summary"`
	IsActive        bool    `json:"is_active"`
	OpensAt         string  `json:"opens_at,omitempty"`
	ClosesAt        string  `json:"closes_at,omitempty"`
	Registrations   int     `json:"registrations"`
	Passed          int     `json:"passed"`
}

// ListExams backs GET /api/admin/certification-exams.
func (h *CertificationHandler) ListExams(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT c.id::text, c.slug, c.title, c.course_id, c.assessment_id::text,
		       COALESCE(a.title, ''), COALESCE(a.status, 'missing'),
		       c.price_paise, c.pass_percent, c.link_valid_days, c.resit_wait_days,
		       c.summary, c.is_active, c.opens_at, c.closes_at,
		       (SELECT count(*) FROM certification_registrations reg
		         WHERE reg.exam_id = c.id AND reg.status <> 'created'),
		       (SELECT count(*) FROM certification_registrations reg
		         WHERE reg.exam_id = c.id AND reg.status = 'passed')
		FROM   certification_exams c
		LEFT   JOIN assessments a ON a.id = c.assessment_id
		ORDER  BY c.is_active DESC, c.title
	`)
	if err != nil {
		h.Log.Error("list certification exams failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load the exams")
		return
	}
	defer rows.Close()

	out := []certExamAdmin{}
	for rows.Next() {
		var e certExamAdmin
		var pricePaise int64
		var opens, closes *time.Time
		if err := rows.Scan(&e.ID, &e.Slug, &e.Title, &e.CourseID, &e.AssessmentID,
			&e.AssessmentTitle, &e.AssessmentState, &pricePaise, &e.PassPercent,
			&e.LinkValidDays, &e.ResitWaitDays, &e.Summary, &e.IsActive, &opens, &closes,
			&e.Registrations, &e.Passed); err != nil {
			h.Log.Error("scan certification exam failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load the exams")
			return
		}
		e.PriceRupees = pricePaise / 100
		if opens != nil {
			e.OpensAt = opens.UTC().Format(time.RFC3339)
		}
		if closes != nil {
			e.ClosesAt = closes.UTC().Format(time.RFC3339)
		}
		out = append(out, e)
	}
	h.write(w, http.StatusOK, map[string]any{"exams": out})
}

// UpsertExam backs POST /api/admin/certification-exams, keyed on slug.
func (h *CertificationHandler) UpsertExam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug          string  `json:"slug"`
		Title         string  `json:"title"`
		CourseID      string  `json:"course_id"`
		AssessmentID  string  `json:"assessment_id"`
		PriceRupees   int64   `json:"price_rupees"`
		PassPercent   float64 `json:"pass_percent"`
		LinkValidDays int     `json:"link_valid_days"`
		ResitWaitDays int     `json:"resit_wait_days"`
		Summary       string  `json:"summary"`
		IsActive      *bool   `json:"is_active"`
		OpensAt       string  `json:"opens_at"`
		ClosesAt      string  `json:"closes_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	slug := strings.ToLower(clip(strings.TrimSpace(req.Slug), 80))
	title := clip(strings.TrimSpace(req.Title), 160)
	if slug == "" || title == "" {
		h.fail(w, http.StatusBadRequest, "slug and title are required")
		return
	}
	if !slugRe.MatchString(slug) {
		h.fail(w, http.StatusBadRequest, "the slug may contain only lowercase letters, numbers and hyphens")
		return
	}
	if req.AssessmentID == "" {
		h.fail(w, http.StatusBadRequest, "choose the paper candidates will sit")
		return
	}
	if req.PriceRupees < 0 {
		h.fail(w, http.StatusBadRequest, "the price cannot be negative")
		return
	}
	if req.PassPercent < 0 || req.PassPercent > 100 {
		h.fail(w, http.StatusBadRequest, "the pass mark must be between 0 and 100")
		return
	}
	if req.LinkValidDays < 1 || req.LinkValidDays > 365 {
		h.fail(w, http.StatusBadRequest, "the link must be valid for between 1 and 365 days")
		return
	}

	// A certification exam may only point at a certification paper. Attaching a
	// practice test would throw a paid exam open to every signed-in student,
	// because practice is the one purpose that is not invite-only.
	var purpose, paperStatus string
	err := h.Pool.QueryRow(r.Context(), `SELECT purpose, status FROM assessments WHERE id = $1::uuid`,
		req.AssessmentID).Scan(&purpose, &paperStatus)
	if errors.Is(err, pgx.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "invalid input syntax")) {
		h.fail(w, http.StatusBadRequest, "that paper no longer exists")
		return
	}
	if err != nil {
		h.Log.Error("look up paper failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not save the exam")
		return
	}
	if !strings.EqualFold(purpose, "certification") {
		h.fail(w, http.StatusBadRequest,
			"that paper's type is '"+purpose+"' — a certification exam needs a paper of type 'certification' so it stays invite-only")
		return
	}

	// parseOptionalTime treats anything unparseable as "no window", so an empty
	// field in the admin form clears the dates rather than failing the save.
	opens, closes := parseOptionalTime(req.OpensAt), parseOptionalTime(req.ClosesAt)
	if opens != nil && closes != nil && closes.Before(*opens) {
		h.fail(w, http.StatusBadRequest, "the window closes before it opens")
		return
	}

	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	resit := req.ResitWaitDays
	if resit < 0 {
		resit = 0
	}

	var id string
	if err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO certification_exams
			(slug, title, course_id, assessment_id, price_paise, pass_percent,
			 link_valid_days, resit_wait_days, summary, is_active, opens_at, closes_at)
		VALUES ($1, $2, $3, $4::uuid, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (slug) DO UPDATE SET
			title = EXCLUDED.title, course_id = EXCLUDED.course_id,
			assessment_id = EXCLUDED.assessment_id, price_paise = EXCLUDED.price_paise,
			pass_percent = EXCLUDED.pass_percent, link_valid_days = EXCLUDED.link_valid_days,
			resit_wait_days = EXCLUDED.resit_wait_days, summary = EXCLUDED.summary,
			is_active = EXCLUDED.is_active, opens_at = EXCLUDED.opens_at,
			closes_at = EXCLUDED.closes_at, updated_at = now()
		RETURNING id::text
	`, slug, title, clip(strings.TrimSpace(req.CourseID), 60), req.AssessmentID,
		req.PriceRupees*100, req.PassPercent, req.LinkValidDays, resit,
		clip(strings.TrimSpace(req.Summary), 2000), active, opens, closes).Scan(&id); err != nil {
		h.Log.Error("upsert certification exam failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not save the exam")
		return
	}

	if paperStatus != "published" {
		h.write(w, http.StatusOK, map[string]any{
			"success": true, "id": id,
			"warning": "saved, but the paper is still a draft — the exam stays off sale until it is published",
		})
		return
	}
	h.write(w, http.StatusOK, map[string]any{"success": true, "id": id})
}

// DeleteExam backs DELETE /api/admin/certification-exams/{id}. Refused once
// anybody has registered: pausing keeps the history, deleting would orphan it.
func (h *CertificationHandler) DeleteExam(w http.ResponseWriter, r *http.Request, id string) {
	var used int
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM certification_registrations WHERE exam_id = $1::uuid`, id).Scan(&used); err != nil {
		h.fail(w, http.StatusBadRequest, "unknown exam")
		return
	}
	if used > 0 {
		h.fail(w, http.StatusConflict,
			fmt.Sprintf("%d people have registered for this exam — pause it instead of deleting it", used))
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM certification_exams WHERE id = $1::uuid`, id)
	if err != nil {
		h.Log.Error("delete certification exam failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not delete the exam")
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, http.StatusNotFound, "exam not found")
		return
	}
	h.write(w, http.StatusOK, map[string]any{"success": true})
}

// ─── Registrations ────────────────────────────────────────────────────────────

type certRegistrationRow struct {
	ID            string   `json:"id"`
	ExamTitle     string   `json:"exam_title"`
	ExamSlug      string   `json:"exam_slug"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	Phone         string   `json:"phone"`
	Status        string   `json:"status"`
	AmountRupees  int64    `json:"amount_rupees"`
	PaymentID     string   `json:"payment_id"`
	CreatedAt     string   `json:"created_at"`
	PaidAt        string   `json:"paid_at,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	ClaimedAt     string   `json:"claimed_at,omitempty"`
	Emailed       bool     `json:"emailed"`
	AttemptID     string   `json:"attempt_id,omitempty"`
	AttemptStatus string   `json:"attempt_status,omitempty"`
	ScorePercent  *float64 `json:"score_percent,omitempty"`
	PassPercent   float64  `json:"pass_percent"`
	IntegrityScr  *int     `json:"integrity_score,omitempty"`
	CredentialID  string   `json:"credential_id,omitempty"`
	Revoked       bool     `json:"credential_revoked"`
	Error         string   `json:"error,omitempty"`
	Notes         string   `json:"notes,omitempty"`
}

const certRegistrationQuery = `
	SELECT reg.id::text, e.title, e.slug, reg.name, reg.email, reg.phone, reg.status,
	       reg.amount_paise, COALESCE(reg.razorpay_payment_id, ''),
	       reg.created_at, reg.paid_at, reg.claim_expires_at, reg.claimed_at,
	       reg.emailed_at IS NOT NULL,
	       COALESCE(at.id::text, ''), COALESCE(at.status, ''),
	       CASE WHEN at.max_score > 0 THEN round((at.score / at.max_score) * 100, 2) END,
	       e.pass_percent, at.integrity_score,
	       COALESCE(c.credential_id, ''), c.revoked_at IS NOT NULL,
	       reg.error, reg.notes
	FROM   certification_registrations reg
	JOIN   certification_exams e ON e.id = reg.exam_id
	LEFT   JOIN certificates c ON c.registration_id = reg.id
	LEFT   JOIN LATERAL (
	           SELECT a.id, a.status, a.score, a.max_score, a.integrity_score
	           FROM   attempts a
	           WHERE  a.assessment_id = e.assessment_id AND a.user_id = reg.user_id
	           ORDER  BY a.started_at DESC
	           LIMIT  1
	       ) at ON true`

func (h *CertificationHandler) scanRegistrations(rows pgx.Rows) ([]certRegistrationRow, error) {
	out := []certRegistrationRow{}
	for rows.Next() {
		var x certRegistrationRow
		var created time.Time
		var paid, expires, claimed *time.Time
		var integrity *int32
		if err := rows.Scan(&x.ID, &x.ExamTitle, &x.ExamSlug, &x.Name, &x.Email, &x.Phone, &x.Status,
			&x.AmountRupees, &x.PaymentID, &created, &paid, &expires, &claimed, &x.Emailed,
			&x.AttemptID, &x.AttemptStatus, &x.ScorePercent, &x.PassPercent, &integrity,
			&x.CredentialID, &x.Revoked, &x.Error, &x.Notes); err != nil {
			return nil, err
		}
		x.AmountRupees /= 100
		x.CreatedAt = created.UTC().Format(time.RFC3339)
		if paid != nil {
			x.PaidAt = paid.UTC().Format(time.RFC3339)
		}
		if expires != nil {
			x.ExpiresAt = expires.UTC().Format(time.RFC3339)
		}
		if claimed != nil {
			x.ClaimedAt = claimed.UTC().Format(time.RFC3339)
		}
		if integrity != nil {
			v := int(*integrity)
			x.IntegrityScr = &v
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ListRegistrations backs GET /api/admin/certifications.
func (h *CertificationHandler) ListRegistrations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}

	where, args := certFilters(q)
	var total int
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT count(*) FROM certification_registrations reg
		JOIN   certification_exams e ON e.id = reg.exam_id `+where, args...).Scan(&total); err != nil {
		h.Log.Error("count certification registrations failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load registrations")
		return
	}

	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf(
		"%s %s ORDER BY reg.created_at DESC LIMIT $%d OFFSET $%d",
		certRegistrationQuery, where, len(args)+1, len(args)+2),
		append(args, size, (page-1)*size)...)
	if err != nil {
		h.Log.Error("list certification registrations failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load registrations")
		return
	}
	defer rows.Close()

	out, err := h.scanRegistrations(rows)
	if err != nil {
		h.Log.Error("scan certification registrations failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load registrations")
		return
	}
	h.write(w, http.StatusOK, map[string]any{
		"registrations": out, "total": total, "page": page, "pageSize": size,
	})
}

func certFilters(q map[string][]string) (string, []any) {
	get := func(k string) string {
		if v, ok := q[k]; ok && len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	var clauses []string
	var args []any
	if s := get("status"); s != "" {
		args = append(args, s)
		clauses = append(clauses, fmt.Sprintf("reg.status = $%d", len(args)))
	}
	if s := get("exam"); s != "" {
		args = append(args, s)
		clauses = append(clauses, fmt.Sprintf("e.slug = $%d", len(args)))
	}
	if s := get("search"); s != "" {
		args = append(args, s)
		clauses = append(clauses, fmt.Sprintf(
			"(reg.name ILIKE '%%' || $%d || '%%' OR reg.email ILIKE '%%' || $%d || '%%')", len(args), len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// ExportRegistrations backs GET /api/admin/certifications/export.csv.
func (h *CertificationHandler) ExportRegistrations(w http.ResponseWriter, r *http.Request) {
	where, args := certFilters(r.URL.Query())
	rows, err := h.Pool.Query(r.Context(), certRegistrationQuery+" "+where+" ORDER BY reg.created_at DESC", args...)
	if err != nil {
		h.Log.Error("export certification registrations failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not export registrations")
		return
	}
	defer rows.Close()
	list, err := h.scanRegistrations(rows)
	if err != nil {
		h.Log.Error("scan certification export failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not export registrations")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="certification-registrations.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("registered_at,exam,name,email,phone,status,amount_rupees,payment_id,attempt_status,score_percent,pass_percent,credential_id\n"))
	for _, x := range list {
		score := ""
		if x.ScorePercent != nil {
			score = strconv.FormatFloat(*x.ScorePercent, 'f', 2, 64)
		}
		_, _ = fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%d,%s,%s,%s,%.2f,%s\n",
			x.CreatedAt, csvCell(x.ExamTitle), csvCell(x.Name), csvCell(x.Email), csvCell(x.Phone),
			x.Status, x.AmountRupees, x.PaymentID, x.AttemptStatus, score, x.PassPercent, x.CredentialID)
	}
}

// csvCell quotes a value that would otherwise break the row.
func csvCell(s string) string {
	if strings.ContainsAny(s, `",`+"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// UpdateRegistration backs PATCH /api/admin/certifications/{id}: notes, a
// recorded refund, and issuing or revoking the credential.
func (h *CertificationHandler) UpdateRegistration(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Notes  *string `json:"notes"`
		Action string  `json:"action"` // refund | issue_certificate | revoke_certificate
		Reason string  `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()

	if req.Notes != nil {
		if _, err := h.Pool.Exec(ctx, `
			UPDATE certification_registrations SET notes = $2, updated_at = now() WHERE id = $1::uuid`,
			id, clip(*req.Notes, 2000)); err != nil {
			h.Log.Error("update certification notes failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not save your note")
			return
		}
	}

	switch req.Action {
	case "":
		// notes-only update

	case "refund":
		// Records the decision; the money is moved in the Razorpay dashboard.
		// The invite is withdrawn so a refunded candidate cannot still sit the
		// exam on the link they already hold.
		tag, err := h.Pool.Exec(ctx, `
			UPDATE certification_registrations
			   SET status = 'refunded', claim_token_hash = NULL, updated_at = now()
			 WHERE id = $1::uuid AND status IN ('paid', 'created', 'expired')`, id)
		if err != nil {
			h.Log.Error("mark refunded failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not record the refund")
			return
		}
		if tag.RowsAffected() == 0 {
			h.fail(w, http.StatusConflict, "only a registration that has not been sat can be refunded here")
			return
		}
		_, _ = h.Pool.Exec(ctx, `
			UPDATE assessment_invites SET status = 'expired', expires_at = now()
			WHERE  id = (SELECT invite_id FROM certification_registrations WHERE id = $1::uuid)`, id)
		// A refunded purchase earns nobody a commission. An unpaid reward is
		// reversed; a paid one is flagged, because no SQL claws back a UPI
		// transfer.
		if h.Referrals != nil {
			h.Referrals.ReverseConversion(ctx, referralKindExam, id, "the exam registration was refunded")
		}

	case "issue_certificate":
		// The manual path for a pass held back by an integrity review.
		var regID, userID, holder, title, attemptID string
		var percent float64
		err := h.Pool.QueryRow(ctx, `
			SELECT reg.id::text, reg.user_id::text, u.name, e.title,
			       COALESCE(at.id::text, ''),
			       COALESCE(CASE WHEN at.max_score > 0 THEN (at.score / at.max_score) * 100 END, 0)
			FROM   certification_registrations reg
			JOIN   certification_exams e ON e.id = reg.exam_id
			JOIN   users u ON u.id = reg.user_id
			LEFT   JOIN LATERAL (
			         SELECT a.id, a.score, a.max_score FROM attempts a
			         WHERE  a.assessment_id = e.assessment_id AND a.user_id = reg.user_id
			         ORDER  BY a.started_at DESC LIMIT 1) at ON true
			WHERE  reg.id = $1::uuid`, id).Scan(&regID, &userID, &holder, &title, &attemptID, &percent)
		if err != nil {
			h.fail(w, http.StatusNotFound, "registration not found")
			return
		}
		cert, err := h.issueCertificate(ctx, regID, userID, holder, title, attemptID, percent)
		if err != nil {
			h.Log.Error("manual certificate issue failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not issue the certificate")
			return
		}
		_, _ = h.Pool.Exec(ctx, `
			UPDATE certification_registrations SET status = 'passed', updated_at = now() WHERE id = $1::uuid`, id)
		h.write(w, http.StatusOK, map[string]any{"success": true, "credential_id": cert.CredentialID})
		return

	case "revoke_certificate":
		reason := clip(strings.TrimSpace(req.Reason), 500)
		if reason == "" {
			h.fail(w, http.StatusBadRequest, "give a reason — it is shown on the public verification page")
			return
		}
		tag, err := h.Pool.Exec(ctx, `
			UPDATE certificates SET revoked_at = now(), revoke_reason = $2
			WHERE  registration_id = $1::uuid AND revoked_at IS NULL`, id, reason)
		if err != nil {
			h.Log.Error("revoke certificate failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not revoke the certificate")
			return
		}
		if tag.RowsAffected() == 0 {
			h.fail(w, http.StatusConflict, "there is no active certificate for this registration")
			return
		}

	default:
		h.fail(w, http.StatusBadRequest, "unknown action")
		return
	}

	h.write(w, http.StatusOK, map[string]any{"success": true})
}

// ResendLink backs POST /api/admin/certifications/{id}/resend. It rotates the
// token, so the old link stops working the moment a new one is issued.
func (h *CertificationHandler) ResendLink(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	var name, email, examTitle, status string
	var expires *time.Time
	var linkValidDays int
	err := h.Pool.QueryRow(ctx, `
		SELECT reg.name, reg.email, e.title, reg.status, reg.claim_expires_at, e.link_valid_days
		FROM   certification_registrations reg
		JOIN   certification_exams e ON e.id = reg.exam_id
		WHERE  reg.id = $1::uuid`, id).Scan(&name, &email, &examTitle, &status, &expires, &linkValidDays)
	if err != nil {
		h.fail(w, http.StatusNotFound, "registration not found")
		return
	}
	if status == "created" {
		h.fail(w, http.StatusConflict, "this registration has not been paid for")
		return
	}
	if certTerminal[status] && status != "expired" {
		h.fail(w, http.StatusConflict, "this exam has already been sat")
		return
	}

	raw, err := randomToken(32)
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not issue a new link")
		return
	}
	newExpiry := time.Now().UTC().Add(time.Duration(linkValidDays) * 24 * time.Hour)
	if _, err := h.Pool.Exec(ctx, `
		UPDATE certification_registrations
		   SET claim_token_hash = $2, claim_expires_at = $3, status = 'paid', updated_at = now()
		 WHERE id = $1::uuid`, id, sha256Hex(raw), newExpiry); err != nil {
		h.Log.Error("rotate certification claim token failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not issue a new link")
		return
	}
	// The invite has its own expiry; move it too, or the engine refuses the
	// paper even though the claim link works.
	_, _ = h.Pool.Exec(ctx, `
		UPDATE assessment_invites SET expires_at = $2, status = 'invited', sent_at = now()
		WHERE  id = (SELECT invite_id FROM certification_registrations WHERE id = $1::uuid)`, id, newExpiry)

	examURL := h.appBase + "/certification/start?t=" + raw
	emailed := false
	if h.Mailer != nil && h.Mailer.enabled() {
		err := h.Mailer.deliver([]string{email}, "", "Your "+examTitle+" exam link",
			certificationResendText(name, examTitle, examURL, newExpiry.Format("2 January 2006")),
			certificationLinkHTML(name, examTitle, examURL, newExpiry.Format("2 January 2006")), email)
		emailed = err == nil
		if err != nil {
			h.Log.Error("certification resend failed", zap.Error(err))
		}
	}
	if emailed {
		_, _ = h.Pool.Exec(ctx, `UPDATE certification_registrations SET emailed_at = now() WHERE id = $1::uuid`, id)
	}

	// The URL is returned to staff, as the scholarship resend does: a counsellor
	// on the phone to a candidate with a broken mailbox needs to read it out.
	h.write(w, http.StatusOK, map[string]any{
		"success": emailed, "email": email, "url": examURL,
		"expires_at": newExpiry.Format(time.RFC3339),
	})
}

// ExtendLink backs POST /api/admin/certifications/{id}/extend — same link, more
// time, for the candidate who asks the day after it lapsed.
func (h *CertificationHandler) ExtendLink(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Days int `json:"days"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req)
	days := req.Days
	if days < 1 || days > 365 {
		days = 30
	}
	until := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)

	tag, err := h.Pool.Exec(r.Context(), `
		UPDATE certification_registrations
		   SET claim_expires_at = $2, status = 'paid', updated_at = now()
		 WHERE id = $1::uuid AND claim_token_hash IS NOT NULL
		   AND status IN ('paid', 'started', 'expired')`, id, until)
	if err != nil {
		h.Log.Error("extend certification link failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not extend the link")
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, http.StatusConflict, "this registration has no live link to extend — resend it instead")
		return
	}
	_, _ = h.Pool.Exec(r.Context(), `
		UPDATE assessment_invites SET expires_at = $2, status = 'invited'
		WHERE  id = (SELECT invite_id FROM certification_registrations WHERE id = $1::uuid)`, id, until)

	h.write(w, http.StatusOK, map[string]any{"success": true, "expires_at": until.Format(time.RFC3339)})
}
