package resolvers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// Dashboard stats, bulk user actions, user export and the admin audit log.
// Kept apart from admin.go so the user CRUD file stays readable.

// ─────────────────────────────────────────────────────────────────────────────
// Audit log
// ─────────────────────────────────────────────────────────────────────────────

// EnsureAuditTable creates the audit table. Called once at startup; a failure
// is logged but not fatal — the admin panel still works, it just stops
// recording who did what.
func (h *AdminHandler) EnsureAuditTable(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS admin_audit_log (
			id          BIGSERIAL PRIMARY KEY,
			actor_id    TEXT        NOT NULL DEFAULT '',
			actor_email TEXT        NOT NULL DEFAULT '',
			action      TEXT        NOT NULL,
			-- The affected user's id and email are copied in rather than
			-- referenced, so the entry survives the user being deleted.
			target_id    TEXT        NOT NULL DEFAULT '',
			target_email TEXT        NOT NULL DEFAULT '',
			detail      JSONB       NOT NULL DEFAULT '{}',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_admin_audit_created ON admin_audit_log(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_admin_audit_action  ON admin_audit_log(action);
	`)
	return err
}

// audit records one admin action. It never fails the request: losing an audit
// row is bad, but refusing to delete a user because the log insert failed is
// worse.
func (h *AdminHandler) audit(ctx context.Context, action, targetID, targetEmail string, detail map[string]interface{}) {
	actorID := middleware.UserIDFromContext(ctx)
	if detail == nil {
		detail = map[string]interface{}{}
	}
	raw, _ := json.Marshal(detail)
	_, err := h.Pool.Exec(context.Background(), `
		INSERT INTO admin_audit_log (actor_id, actor_email, action, target_id, target_email, detail)
		VALUES ($1, COALESCE((SELECT email FROM users WHERE id::text = $1), ''), $2, $3, $4, $5::jsonb)
	`, actorID, action, targetID, targetEmail, string(raw))
	if err != nil {
		h.Log.Warn("admin audit insert failed", zap.String("action", action), zap.Error(err))
	}
}

// emailOf looks up a user's email for the audit trail. Must run before a delete.
func (h *AdminHandler) emailOf(ctx context.Context, userID string) string {
	var email string
	_ = h.Pool.QueryRow(ctx, `SELECT email FROM users WHERE id::text = $1`, userID).Scan(&email)
	return email
}

type auditRow struct {
	ID          int64           `json:"id"`
	ActorEmail  string          `json:"actor_email"`
	Action      string          `json:"action"`
	TargetID    string          `json:"target_id"`
	TargetEmail string          `json:"target_email"`
	Detail      json.RawMessage `json:"detail"`
	CreatedAt   string          `json:"created_at"`
}

// GET /api/admin/audit?page&page_size&action&search
func (h *AdminHandler) handleListAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	where := []string{"TRUE"}
	args := []interface{}{}
	if a := q.Get("action"); a != "" {
		args = append(args, a)
		where = append(where, fmt.Sprintf("action = $%d", len(args)))
	}
	if s := strings.TrimSpace(q.Get("search")); s != "" {
		args = append(args, "%"+s+"%")
		where = append(where, fmt.Sprintf("(actor_email ILIKE $%d OR target_email ILIKE $%d)", len(args), len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := h.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM admin_audit_log WHERE `+cond, args...).Scan(&total); err != nil {
		h.Log.Error("count audit failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to load audit log")
		return
	}

	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf(`
		SELECT id, actor_email, action, target_id, target_email, detail, created_at
		FROM admin_audit_log WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, cond, len(args)-1, len(args)), args...)
	if err != nil {
		h.Log.Error("list audit failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to load audit log")
		return
	}
	defer rows.Close()

	entries := []auditRow{}
	for rows.Next() {
		var e auditRow
		var detail []byte
		var at time.Time
		if err := rows.Scan(&e.ID, &e.ActorEmail, &e.Action, &e.TargetID, &e.TargetEmail, &detail, &at); err != nil {
			h.Log.Error("scan audit failed", zap.Error(err))
			h.jsonErr(w, http.StatusInternalServerError, "failed to load audit log")
			return
		}
		e.Detail = detail
		e.CreatedAt = at.Format(time.RFC3339)
		entries = append(entries, e)
	}
	h.json(w, http.StatusOK, map[string]interface{}{
		"entries": entries, "total": total, "page": page, "pageSize": pageSize,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Dashboard
// ─────────────────────────────────────────────────────────────────────────────

type dayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// GET /api/admin/stats — headline numbers for the dashboard. Each figure is
// queried on its own and a failure leaves it at zero: the enquiry, scholarship
// and attendance tables only exist when their handlers initialised, and one
// missing table should not blank the whole dashboard.
func (h *AdminHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	count := func(label, q string) int {
		var n int
		if err := h.Pool.QueryRow(ctx, q).Scan(&n); err != nil {
			h.Log.Debug("dashboard stat unavailable", zap.String("stat", label), zap.Error(err))
		}
		return n
	}

	byRole := map[string]int{}
	if rows, err := h.Pool.Query(ctx, `SELECT role, COUNT(*) FROM users WHERE role <> 'applicant' GROUP BY role`); err == nil {
		for rows.Next() {
			var role string
			var n int
			if rows.Scan(&role, &n) == nil {
				byRole[role] = n
			}
		}
		rows.Close()
	}

	byCourse := map[string]int{}
	if rows, err := h.Pool.Query(ctx, `SELECT course_id, COUNT(*) FROM user_courses GROUP BY course_id`); err == nil {
		for rows.Next() {
			var c string
			var n int
			if rows.Scan(&c, &n) == nil {
				byCourse[c] = n
			}
		}
		rows.Close()
	}

	// Sign-ups per day for the last 30 days, zero-filled so the chart has no gaps.
	signups := []dayCount{}
	if rows, err := h.Pool.Query(ctx, `
		SELECT d::date::text, COUNT(u.id)
		FROM generate_series((now() - interval '29 days')::date, now()::date, interval '1 day') d
		LEFT JOIN users u ON u.created_at::date = d::date AND u.role <> 'applicant'
		GROUP BY d ORDER BY d`); err == nil {
		for rows.Next() {
			var dc dayCount
			if rows.Scan(&dc.Day, &dc.Count) == nil {
				signups = append(signups, dc)
			}
		}
		rows.Close()
	}

	total := 0
	for _, n := range byRole {
		total += n
	}

	h.json(w, http.StatusOK, map[string]interface{}{
		"users_total":         total,
		"users_by_role":       byRole,
		"users_by_course":     byCourse,
		"users_new_7d":        count("users_new_7d", `SELECT COUNT(*) FROM users WHERE role <> 'applicant' AND created_at > now() - interval '7 days'`),
		"users_no_course":     count("users_no_course", `SELECT COUNT(*) FROM users u WHERE u.role = 'student' AND NOT EXISTS (SELECT 1 FROM user_courses uc WHERE uc.user_id = u.id)`),
		"enquiries_new":       count("enquiries_new", `SELECT COUNT(*) FROM inquiries WHERE status = 'new'`),
		"enquiries_7d":        count("enquiries_7d", `SELECT COUNT(*) FROM inquiries WHERE created_at > now() - interval '7 days'`),
		"scholarship_pending": count("scholarship_pending", `SELECT COUNT(*) FROM scholarship_applications WHERE status IN ('submitted', 'evaluated')`),
		"scholarship_7d":      count("scholarship_7d", `SELECT COUNT(*) FROM scholarship_applications WHERE created_at > now() - interval '7 days'`),
		"tests_published":     count("tests_published", `SELECT COUNT(*) FROM assessments WHERE status = 'published'`),
		"attempts_7d":         count("attempts_7d", `SELECT COUNT(*) FROM attempts WHERE started_at > now() - interval '7 days'`),
		"attempts_live":       count("attempts_live", `SELECT COUNT(*) FROM attempts WHERE status = 'in_progress' AND expires_at > now()`),
		"answers_to_grade":    count("answers_to_grade", `SELECT COUNT(*) FROM attempt_questions aq JOIN attempts a ON a.id = aq.attempt_id WHERE aq.kind = 'descriptive' AND aq.grading_status IN ('pending', 'manual_review') AND a.status <> 'in_progress'`),
		"classes_active":      count("classes_active", `SELECT COUNT(*) FROM class_schedules`),
		"signups_30d":         signups,
	})
}

// GET /api/admin/grading-queue — descriptive answers waiting for a mark, so
// the dashboard can link straight to the attempt that needs grading.
func (h *AdminHandler) handleGradingQueue(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT a.id::text, a.assessment_id::text, s.title, COALESCE(u.name, ''), COALESCE(u.email, ''),
		       COUNT(*)::int, MIN(COALESCE(a.submitted_at, a.started_at))
		FROM attempt_questions aq
		JOIN attempts a    ON a.id = aq.attempt_id
		JOIN assessments s ON s.id = a.assessment_id
		LEFT JOIN users u  ON u.id = a.user_id
		WHERE aq.kind = 'descriptive' AND aq.grading_status IN ('pending', 'manual_review')
		  AND a.status <> 'in_progress'
		GROUP BY a.id, a.assessment_id, s.title, u.name, u.email
		ORDER BY 7 ASC
		LIMIT 50`)
	if err != nil {
		h.Log.Error("grading queue failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to load grading queue")
		return
	}
	defer rows.Close()

	type item struct {
		AttemptID    string `json:"attempt_id"`
		AssessmentID string `json:"assessment_id"`
		Title        string `json:"title"`
		UserName     string `json:"user_name"`
		UserEmail    string `json:"user_email"`
		Pending      int    `json:"pending"`
		SubmittedAt  string `json:"submitted_at"`
	}
	items := []item{}
	for rows.Next() {
		var it item
		var at time.Time
		if err := rows.Scan(&it.AttemptID, &it.AssessmentID, &it.Title, &it.UserName, &it.UserEmail, &it.Pending, &at); err != nil {
			h.Log.Error("scan grading queue failed", zap.Error(err))
			continue
		}
		it.SubmittedAt = at.Format(time.RFC3339)
		items = append(items, it)
	}
	h.json(w, http.StatusOK, map[string]interface{}{"items": items})
}

// ─────────────────────────────────────────────────────────────────────────────
// Bulk actions
// ─────────────────────────────────────────────────────────────────────────────

// POST /api/admin/users/bulk  {ids, action, role?, course_id?}
// action: set_role | grant_course | revoke_course | delete
func (h *AdminHandler) handleBulkUsers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs      []string `json:"ids"`
		Action   string   `json:"action"`
		Role     string   `json:"role"`
		CourseID string   `json:"course_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		h.jsonErr(w, http.StatusBadRequest, "no users selected")
		return
	}
	if len(req.IDs) > 1000 {
		h.jsonErr(w, http.StatusBadRequest, "at most 1000 users per bulk action")
		return
	}

	// An admin demoting or deleting themselves in a bulk selection is almost
	// always a mis-click, and it locks them out mid-task. Refuse it outright.
	self := middleware.UserIDFromContext(r.Context())
	if req.Action == "delete" || (req.Action == "set_role" && req.Role != "admin") {
		for _, id := range req.IDs {
			if id == self {
				h.jsonErr(w, http.StatusBadRequest, "you cannot delete or demote your own account")
				return
			}
		}
	}

	ctx := r.Context()
	var (
		sql  string
		args []interface{}
	)
	switch req.Action {
	case "set_role":
		if req.Role != "student" && req.Role != "admin" && req.Role != "recruiter" {
			h.jsonErr(w, http.StatusBadRequest, "role must be 'student', 'recruiter' or 'admin'")
			return
		}
		sql = `UPDATE users SET role = $2, updated_at = now() WHERE id::text = ANY($1) AND role <> 'applicant'
		       RETURNING id::text, email`
		args = []interface{}{req.IDs, req.Role}
	case "grant_course":
		if req.CourseID == "" {
			h.jsonErr(w, http.StatusBadRequest, "course_id is required")
			return
		}
		sql = `INSERT INTO user_courses (user_id, course_id)
		       SELECT u.id, $2 FROM users u WHERE u.id::text = ANY($1)
		       ON CONFLICT (user_id, course_id) DO NOTHING
		       RETURNING user_id::text, (SELECT email FROM users WHERE id = user_id)`
		args = []interface{}{req.IDs, req.CourseID}
	case "revoke_course":
		if req.CourseID == "" {
			h.jsonErr(w, http.StatusBadRequest, "course_id is required")
			return
		}
		sql = `DELETE FROM user_courses uc USING users u
		       WHERE uc.user_id = u.id AND u.id::text = ANY($1) AND uc.course_id = $2
		       RETURNING u.id::text, u.email`
		args = []interface{}{req.IDs, req.CourseID}
	case "delete":
		sql = `DELETE FROM users WHERE id::text = ANY($1) AND role <> 'applicant' RETURNING id::text, email`
		args = []interface{}{req.IDs}
	default:
		h.jsonErr(w, http.StatusBadRequest, "unknown action")
		return
	}

	rows, err := h.Pool.Query(ctx, sql, args...)
	if err != nil {
		h.Log.Error("bulk user action failed", zap.String("action", req.Action), zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "bulk action failed")
		return
	}
	type affected struct{ id, email string }
	var done []affected
	for rows.Next() {
		var a affected
		if rows.Scan(&a.id, &a.email) == nil {
			done = append(done, a)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		h.Log.Error("bulk user action failed", zap.String("action", req.Action), zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "bulk action failed")
		return
	}

	auditAction := map[string]string{
		"set_role": "user.role_changed", "grant_course": "course.granted",
		"revoke_course": "course.revoked", "delete": "user.deleted",
	}[req.Action]
	detail := map[string]interface{}{"bulk": true}
	if req.Role != "" {
		detail["role"] = req.Role
	}
	if req.CourseID != "" {
		detail["course_id"] = req.CourseID
	}
	for _, a := range done {
		h.audit(ctx, auditAction, a.id, a.email, detail)
	}

	h.json(w, http.StatusOK, map[string]interface{}{"affected": len(done), "requested": len(req.IDs)})
}

// ─────────────────────────────────────────────────────────────────────────────
// Export
// ─────────────────────────────────────────────────────────────────────────────

// GET /api/admin/users/export.csv — same filters as the list, no paging.
func (h *AdminHandler) handleExportUsers(w http.ResponseWriter, r *http.Request) {
	f := userFiltersFromQuery(r)
	cond, args := f.where()
	rows, err := h.Pool.Query(r.Context(), `
		SELECT u.name, u.email, COALESCE(p.phone, ''), u.role, u.created_at,
		       COALESCE(string_agg(uc.course_id, ';' ORDER BY uc.course_id), '')
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id = u.id
		LEFT JOIN user_courses uc ON uc.user_id = u.id
		WHERE `+cond+`
		GROUP BY u.id, u.name, u.email, p.phone, u.role, u.created_at
		ORDER BY u.created_at DESC`, args...)
	if err != nil {
		h.Log.Error("export users failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to export users")
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="users.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"name", "email", "phone", "role", "created_at", "courses"})
	n := 0
	for rows.Next() {
		var name, email, phone, role, courses string
		var created time.Time
		if err := rows.Scan(&name, &email, &phone, &role, &created, &courses); err != nil {
			continue
		}
		_ = cw.Write([]string{csvSafe(name), email, csvSafe(phone), role, created.Format(time.RFC3339), courses})
		n++
	}
	cw.Flush()
	h.audit(r.Context(), "users.exported", "", "", map[string]interface{}{"rows": n})
}

// csvSafe defuses spreadsheet formula injection: a cell starting with = + - @
// is executed by Excel when the file is opened.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@", rune(s[0])) {
		return "'" + s
	}
	return s
}

// ─────────────────────────────────────────────────────────────────────────────
// Filters shared by list and export
// ─────────────────────────────────────────────────────────────────────────────

type userFilters struct {
	Search   string
	Role     string
	CourseID string // "none" = students with no course
	From, To string // YYYY-MM-DD, inclusive
}

func userFiltersFromQuery(r *http.Request) userFilters {
	q := r.URL.Query()
	return userFilters{
		Search:   strings.TrimSpace(q.Get("search")),
		Role:     q.Get("role"),
		CourseID: q.Get("course"),
		From:     q.Get("from"),
		To:       q.Get("to"),
	}
}

// where builds the WHERE clause over alias `u`. Placeholders start at $1.
// Scholarship applicants are always excluded — see listUsers.
func (f userFilters) where() (string, []interface{}) {
	conds := []string{`u.role <> 'applicant'`}
	args := []interface{}{}
	next := func(v interface{}) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.Search != "" {
		p := next("%" + f.Search + "%")
		conds = append(conds, fmt.Sprintf("(u.email ILIKE %s OR u.name ILIKE %s)", p, p))
	}
	if f.Role != "" {
		conds = append(conds, "u.role = "+next(f.Role))
	}
	switch f.CourseID {
	case "":
	case "none":
		conds = append(conds, "NOT EXISTS (SELECT 1 FROM user_courses x WHERE x.user_id = u.id)")
	default:
		conds = append(conds, "EXISTS (SELECT 1 FROM user_courses x WHERE x.user_id = u.id AND x.course_id = "+next(f.CourseID)+")")
	}
	if _, err := time.Parse("2006-01-02", f.From); err == nil {
		conds = append(conds, "u.created_at >= "+next(f.From)+"::date")
	}
	if _, err := time.Parse("2006-01-02", f.To); err == nil {
		conds = append(conds, "u.created_at < "+next(f.To)+"::date + 1")
	}
	return strings.Join(conds, " AND "), args
}
