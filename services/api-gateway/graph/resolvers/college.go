package resolvers

// College view — a college's placement cell (TPO) sees the progress of its own
// students, read-only.
//
// Colleges live in their own tables rather than in `companies`. Several
// recruiter checks accept any company_members row regardless of its role, so a
// college staff row there would have let a placement officer into recruiter
// data. Keeping them apart makes that impossible rather than merely unlikely.
//
// A student joins a college in one of two ways, both their own choice:
//   - by entering the college's join code in their profile, or
//   - by accepting a pending request the college created from a student list.
// They can unlink at any time, and the college then sees nothing of them.
//
// What the college sees is deliberately narrow: progress, attendance, test
// scores and certificates. Never answers, camera data or messages. Every list
// view and export is written to college_access_log.

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// courseTotals is each course's syllabus length, keyed by the progress key the
// course page records under. It mirrors `totalItems` in skillofied-app's
// src/lib/courseRegistry.ts, which is the source of truth — update both.
var courseTotals = map[string]struct {
	Title string
	Total int
}{
	"maxGolangIndexRead":           {"Golang Engineering", 179},
	"maxGenAIIndexRead":            {"GenAI & Forward Deployed Engineering", 110},
	"maxFullStackIndexRead":        {"Full Stack Engineering", 7},
	"maxIndexRead":                 {"Front-End Technologies", 201},
	"maxJavaIndexRead":             {"Java Development", 210},
	"maxSqlIndexRead":              {"Mastering SQL", 91},
	"maxOsIndexRead":               {"Operating Systems", 107},
	"maxNetworkingIndexRead":       {"Computer Networks", 105},
	"maxSeoIndexRead":              {"SEO Fundamentals", 42},
	"maxDigitalMarketingIndexRead": {"Digital Marketing Strategy", 47},
	"maxTestingIndexRead":          {"Software Testing", 168},
}

const (
	collegeRoleStaff   = "tpo"
	maxRosterRows      = 2000
	inactiveAfterDays  = 14
	lowAttendancePct   = 60
	attendanceDays     = 30
	integrityFlagBelow = 50
)

type CollegeHandler struct {
	Pool       *pgxpool.Pool
	Log        *zap.Logger
	Attendance *AttendanceHandler
	codeLimit  *rateLimiter
	now        func() time.Time
}

func NewCollegeHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger, att *AttendanceHandler) (*CollegeHandler, error) {
	h := &CollegeHandler{
		Pool: pool, Log: log, Attendance: att, now: time.Now,
		// Join codes are guessable in principle; ten tries per ten minutes per
		// network keeps that theoretical.
		codeLimit: newRateLimiter(10, 10*time.Minute),
	}
	if err := h.ensureTables(ctx); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *CollegeHandler) ensureTables(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS colleges (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name       TEXT NOT NULL,
			city       TEXT NOT NULL DEFAULT '',
			-- What students type to link themselves. Upper-case, unambiguous letters.
			join_code  TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);

		CREATE TABLE IF NOT EXISTS college_staff (
			college_id UUID NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			added_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (college_id, user_id)
		);

		CREATE TABLE IF NOT EXISTS college_students (
			college_id UUID NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			roll_no    TEXT NOT NULL DEFAULT '',
			branch     TEXT NOT NULL DEFAULT '',
			year       TEXT NOT NULL DEFAULT '',
			-- 'pending': the college listed them, they have not said yes yet.
			status     TEXT NOT NULL CHECK (status IN ('pending', 'linked')),
			invited_at TIMESTAMPTZ,
			linked_at  TIMESTAMPTZ,
			PRIMARY KEY (college_id, user_id)
		);
		-- A student belongs to at most one college at a time.
		CREATE UNIQUE INDEX IF NOT EXISTS college_students_one_linked
			ON college_students (user_id) WHERE status = 'linked';

		CREATE TABLE IF NOT EXISTS college_access_log (
			id         BIGSERIAL PRIMARY KEY,
			college_id UUID NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
			viewer_id  UUID NOT NULL,
			action     TEXT NOT NULL,
			detail     TEXT NOT NULL DEFAULT '',
			at         TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS college_access_log_college ON college_access_log (college_id, at DESC);
	`)
	return err
}

// ServeHTTP routes /api/college/*. Every path needs a session (the gateway's
// auth middleware); roles are checked per route below.
func (h *CollegeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.UserIDFromContext(ctx)
	role := middleware.RoleFromContext(ctx)
	if userID == "" {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/college"), "/")
	parts := strings.Split(path, "/")

	switch {
	// ── Student side ──
	case path == "link" && role == "student":
		switch r.Method {
		case http.MethodGet:
			h.myLink(w, r, userID)
		case http.MethodPost:
			h.linkByCode(w, r, userID)
		case http.MethodDelete:
			h.unlink(w, r, userID)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case len(parts) == 3 && parts[0] == "requests" && role == "student" && r.Method == http.MethodPost:
		h.answerRequest(w, r, userID, parts[1], parts[2])
	case path == "link" || parts[0] == "requests":
		writeErr(w, http.StatusForbidden, "only student accounts can link to a college")

	// ── Admin side ──
	case parts[0] == "admin":
		if role != "admin" {
			writeErr(w, http.StatusForbidden, "admin only")
			return
		}
		h.admin(w, r, userID, parts[1:])

	// ── Placement cell ──
	default:
		if role != collegeRoleStaff {
			writeErr(w, http.StatusForbidden, "college staff only")
			return
		}
		collegeID, collegeName, joinCode, err := h.staffCollege(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusForbidden, "your account is not attached to a college yet — ask Knovate to add you")
			return
		}
		if err != nil {
			h.Log.Error("load staff college failed", zap.Error(err))
			writeErr(w, http.StatusInternalServerError, "could not load your college")
			return
		}
		switch {
		case path == "overview" && r.Method == http.MethodGet:
			h.overview(w, r, userID, collegeID, collegeName, joinCode)
		case path == "students" && r.Method == http.MethodGet:
			h.students(w, r, userID, collegeID, collegeName)
		case path == "roster" && r.Method == http.MethodPost:
			h.roster(w, r, userID, collegeID)
		default:
			writeErr(w, http.StatusNotFound, "not found")
		}
	}
}

// staffCollege is the college a staff member belongs to. The college id always
// comes from here — never from the client.
func (h *CollegeHandler) staffCollege(ctx context.Context, userID string) (id, name, code string, err error) {
	err = h.Pool.QueryRow(ctx, `
		SELECT c.id::text, c.name, c.join_code
		FROM   college_staff s JOIN colleges c ON c.id = s.college_id
		WHERE  s.user_id = $1::uuid
		ORDER  BY s.added_at LIMIT 1
	`, userID).Scan(&id, &name, &code)
	return
}

func (h *CollegeHandler) logAccess(ctx context.Context, collegeID, viewerID, action, detail string) {
	if _, err := h.Pool.Exec(ctx, `
		INSERT INTO college_access_log (college_id, viewer_id, action, detail) VALUES ($1::uuid, $2::uuid, $3, $4)
	`, collegeID, viewerID, action, clip(detail, 300)); err != nil {
		h.Log.Warn("college access log failed", zap.Error(err))
	}
}

// ─── Student side ─────────────────────────────────────────────────────────────

type linkDetails struct {
	RollNo string `json:"rollNo"`
	Branch string `json:"branch"`
	Year   string `json:"year"`
}

func (d *linkDetails) clean() {
	d.RollNo = clip(d.RollNo, 40)
	d.Branch = clip(d.Branch, 60)
	d.Year = clip(d.Year, 30)
}

// GET /api/college/link → the student's college (if any) and pending requests.
func (h *CollegeHandler) myLink(w http.ResponseWriter, r *http.Request, userID string) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT c.id::text, c.name, c.city, s.status, s.roll_no, s.branch, s.year, s.linked_at
		FROM   college_students s JOIN colleges c ON c.id = s.college_id
		WHERE  s.user_id = $1::uuid
		ORDER  BY s.status DESC, s.invited_at DESC NULLS LAST
	`, userID)
	if err != nil {
		h.Log.Error("load college link failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not load your college")
		return
	}
	defer rows.Close()
	type entry struct {
		CollegeID string     `json:"collegeId"`
		Name      string     `json:"name"`
		City      string     `json:"city"`
		RollNo    string     `json:"rollNo"`
		Branch    string     `json:"branch"`
		Year      string     `json:"year"`
		LinkedAt  *time.Time `json:"linkedAt,omitempty"`
	}
	var linked *entry
	pending := []entry{}
	for rows.Next() {
		var e entry
		var status string
		if err := rows.Scan(&e.CollegeID, &e.Name, &e.City, &status, &e.RollNo, &e.Branch, &e.Year, &e.LinkedAt); err != nil {
			h.Log.Error("scan college link failed", zap.Error(err))
			writeErr(w, http.StatusInternalServerError, "could not load your college")
			return
		}
		if status == "linked" {
			cp := e
			linked = &cp
		} else {
			pending = append(pending, e)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"college": linked, "requests": pending})
}

// POST /api/college/link {code, rollNo, branch, year}
func (h *CollegeHandler) linkByCode(w http.ResponseWriter, r *http.Request, userID string) {
	var body struct {
		Code string `json:"code"`
		linkDetails
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.clean()
	code := normaliseJoinCode(body.Code)
	if code == "" {
		writeErr(w, http.StatusBadRequest, "enter your college's join code")
		return
	}
	if !h.codeLimit.allow("college-code:" + userID) {
		writeErr(w, http.StatusTooManyRequests, "too many tries — wait a few minutes and try again")
		return
	}
	ctx := r.Context()
	var collegeID, name string
	err := h.Pool.QueryRow(ctx, `SELECT id::text, name FROM colleges WHERE join_code = $1`, code).Scan(&collegeID, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "that code does not match a college — check it with your placement cell")
		return
	}
	if err != nil {
		h.Log.Error("look up join code failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not link your account, please try again")
		return
	}
	h.link(w, r, userID, collegeID, name, body.linkDetails, true)
}

// POST /api/college/requests/{collegeId}/accept|decline
func (h *CollegeHandler) answerRequest(w http.ResponseWriter, r *http.Request, userID, collegeID, answer string) {
	ctx := r.Context()
	var name string
	var d linkDetails
	err := h.Pool.QueryRow(ctx, `
		SELECT c.name, s.roll_no, s.branch, s.year
		FROM   college_students s JOIN colleges c ON c.id = s.college_id
		WHERE  s.college_id = $1::uuid AND s.user_id = $2::uuid AND s.status = 'pending'
	`, collegeID, userID).Scan(&name, &d.RollNo, &d.Branch, &d.Year)
	if errors.Is(err, pgx.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "invalid input syntax")) {
		writeErr(w, http.StatusNotFound, "this request is no longer open")
		return
	}
	if err != nil {
		h.Log.Error("load college request failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not answer the request, please try again")
		return
	}
	switch answer {
	case "accept":
		h.link(w, r, userID, collegeID, name, d, false)
	case "decline":
		if _, err := h.Pool.Exec(ctx, `
			DELETE FROM college_students WHERE college_id = $1::uuid AND user_id = $2::uuid AND status = 'pending'
		`, collegeID, userID); err != nil {
			h.Log.Error("decline college request failed", zap.Error(err))
			writeErr(w, http.StatusInternalServerError, "could not answer the request, please try again")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"declined": true})
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

// link makes the student a linked member of the college. When the student typed
// the details themselves (byCode) those win over what the college listed.
func (h *CollegeHandler) link(w http.ResponseWriter, r *http.Request, userID, collegeID, name string, d linkDetails, byCode bool) {
	ctx := r.Context()
	var other string
	err := h.Pool.QueryRow(ctx, `
		SELECT c.name FROM college_students s JOIN colleges c ON c.id = s.college_id
		WHERE s.user_id = $1::uuid AND s.status = 'linked' AND s.college_id <> $2::uuid
	`, userID, collegeID).Scan(&other)
	if err == nil {
		writeErr(w, http.StatusConflict, "your account is linked to "+other+" — unlink it first in your profile")
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("check existing college link failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not link your account, please try again")
		return
	}
	if _, err := h.Pool.Exec(ctx, `
		INSERT INTO college_students (college_id, user_id, roll_no, branch, year, status, linked_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'linked', now())
		ON CONFLICT (college_id, user_id) DO UPDATE
		   SET status    = 'linked',
		       linked_at = COALESCE(college_students.linked_at, now()),
		       roll_no   = CASE WHEN $6 AND EXCLUDED.roll_no <> '' THEN EXCLUDED.roll_no ELSE college_students.roll_no END,
		       branch    = CASE WHEN $6 AND EXCLUDED.branch  <> '' THEN EXCLUDED.branch  ELSE college_students.branch  END,
		       year      = CASE WHEN $6 AND EXCLUDED.year    <> '' THEN EXCLUDED.year    ELSE college_students.year    END
	`, collegeID, userID, d.RollNo, d.Branch, d.Year, byCode); err != nil {
		h.Log.Error("link college failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not link your account, please try again")
		return
	}
	// Any other college's pending request is moot now.
	_, _ = h.Pool.Exec(ctx, `DELETE FROM college_students WHERE user_id = $1::uuid AND status = 'pending'`, userID)
	writeJSON(w, http.StatusOK, map[string]any{"linked": true, "collegeId": collegeID, "name": name})
}

// DELETE /api/college/link — the college stops seeing the student at once.
func (h *CollegeHandler) unlink(w http.ResponseWriter, r *http.Request, userID string) {
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM college_students WHERE user_id = $1::uuid AND status = 'linked'`, userID)
	if err != nil {
		h.Log.Error("unlink college failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not unlink, please try again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unlinked": tag.RowsAffected() > 0})
}

// ─── Placement cell ───────────────────────────────────────────────────────────

type collegeStudentRow struct {
	UserID       string     `json:"userId"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	RollNo       string     `json:"rollNo"`
	Branch       string     `json:"branch"`
	Year         string     `json:"year"`
	LinkedAt     *time.Time `json:"linkedAt"`
	LastActiveAt *time.Time `json:"lastActiveAt"`
	// The course they touched most recently, and how far through it they are.
	CourseTitle     string `json:"courseTitle"`
	CoursePercent   int    `json:"coursePercent"`
	AttendancePct   *int   `json:"attendancePercent"`
	AttendanceTotal int    `json:"attendanceSessions"`
	LatestTest      *struct {
		Title          string  `json:"title"`
		Percent        float64 `json:"percent"`
		EndedByProctor bool    `json:"endedByProctor"`
	} `json:"latestTest"`
	IntegrityFlag    bool   `json:"integrityFlag"`
	Certificates     int    `json:"certificates"`
	CertificateTitle string `json:"certificateTitle"`
	// Inactive, low attendance — the "needs attention" buckets.
	Inactive      bool `json:"inactive"`
	LowAttendance bool `json:"lowAttendance"`
}

// loadStudents builds every linked student's row. Fine for a college of a few
// thousand: a handful of set queries plus one attendance pass per student.
func (h *CollegeHandler) loadStudents(ctx context.Context, collegeID string) ([]*collegeStudentRow, error) {
	rows, err := h.Pool.Query(ctx, `
		SELECT u.id::text, u.name, u.email, s.roll_no, s.branch, s.year, s.linked_at
		FROM   college_students s JOIN users u ON u.id = s.user_id
		WHERE  s.college_id = $1::uuid AND s.status = 'linked'
		ORDER  BY u.name
	`, collegeID)
	if err != nil {
		return nil, err
	}
	var out []*collegeStudentRow
	byID := map[string]*collegeStudentRow{}
	var ids []string
	for rows.Next() {
		s := &collegeStudentRow{}
		if err := rows.Scan(&s.UserID, &s.Name, &s.Email, &s.RollNo, &s.Branch, &s.Year, &s.LinkedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, s)
		byID[s.UserID] = s
		ids = append(ids, s.UserID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return out, nil
	}
	touch := func(s *collegeStudentRow, t *time.Time) {
		if t != nil && (s.LastActiveAt == nil || t.After(*s.LastActiveAt)) {
			tt := *t
			s.LastActiveAt = &tt
		}
	}

	// Course progress: the most recently touched course is the one shown.
	pr, err := h.Pool.Query(ctx, `
		SELECT user_id, course_key, max_index, updated_at FROM course_progress
		WHERE  user_id = ANY($1::text[]) ORDER BY updated_at
	`, ids)
	if err != nil {
		return nil, err
	}
	for pr.Next() {
		var uid, key string
		var idx int
		var at time.Time
		if err := pr.Scan(&uid, &key, &idx, &at); err != nil {
			pr.Close()
			return nil, err
		}
		s, c := byID[uid], courseTotals[key]
		if s == nil || c.Total == 0 {
			continue
		}
		s.CourseTitle = c.Title
		s.CoursePercent = min(100, (idx+1)*100/c.Total)
		touch(s, &at)
	}
	pr.Close()

	// Latest finished test, and whether any test raised an integrity flag.
	tr, err := h.Pool.Query(ctx, `
		SELECT DISTINCT ON (at.user_id) at.user_id::text, a.title,
		       CASE WHEN at.max_score > 0 THEN at.score / at.max_score * 100 ELSE 0 END,
		       at.status = 'disqualified',
		       COALESCE(at.submitted_at, at.started_at),
		       EXISTS (SELECT 1 FROM attempts x WHERE x.user_id = at.user_id
		                 AND (x.status = 'disqualified' OR x.integrity_score < $2))
		FROM   attempts at JOIN assessments a ON a.id = at.assessment_id
		WHERE  at.user_id = ANY($1::uuid[]) AND at.status IN ('evaluated', 'disqualified')
		ORDER  BY at.user_id, COALESCE(at.submitted_at, at.started_at) DESC
	`, ids, integrityFlagBelow)
	if err != nil {
		return nil, err
	}
	for tr.Next() {
		var uid, title string
		var pct float64
		var ended, flag bool
		var at *time.Time
		if err := tr.Scan(&uid, &title, &pct, &ended, &at, &flag); err != nil {
			tr.Close()
			return nil, err
		}
		if s := byID[uid]; s != nil {
			s.LatestTest = &struct {
				Title          string  `json:"title"`
				Percent        float64 `json:"percent"`
				EndedByProctor bool    `json:"endedByProctor"`
			}{title, roundTo(pct, 1), ended}
			s.IntegrityFlag = flag
			touch(s, at)
		}
	}
	tr.Close()

	// Any test started counts as activity, finished or not.
	ar, err := h.Pool.Query(ctx, `
		SELECT user_id::text, max(started_at) FROM attempts WHERE user_id = ANY($1::uuid[]) GROUP BY user_id
	`, ids)
	if err != nil {
		return nil, err
	}
	for ar.Next() {
		var uid string
		var at *time.Time
		if err := ar.Scan(&uid, &at); err != nil {
			ar.Close()
			return nil, err
		}
		if s := byID[uid]; s != nil {
			touch(s, at)
		}
	}
	ar.Close()

	// Certificates still standing.
	cr, err := h.Pool.Query(ctx, `
		SELECT DISTINCT ON (user_id) user_id::text, title,
		       count(*) OVER (PARTITION BY user_id)
		FROM   certificates WHERE user_id = ANY($1::uuid[]) AND revoked_at IS NULL
		ORDER  BY user_id, issued_at DESC
	`, ids)
	if err != nil {
		return nil, err
	}
	for cr.Next() {
		var uid, title string
		var n int
		if err := cr.Scan(&uid, &title, &n); err != nil {
			cr.Close()
			return nil, err
		}
		if s := byID[uid]; s != nil {
			s.Certificates, s.CertificateTitle = n, title
		}
	}
	cr.Close()

	// Attendance over the last 30 days, computed exactly as the student's own
	// attendance page does.
	now := h.now()
	if h.Attendance != nil {
		to := dayStart(now)
		from := to.AddDate(0, 0, -(attendanceDays - 1))
		for _, s := range out {
			scheds, err := h.Attendance.studentSchedules(ctx, s.UserID)
			if err != nil {
				return nil, err
			}
			if len(scheds) == 0 {
				continue
			}
			marked, err := h.Attendance.markedIn(ctx, s.UserID, from.Format("2006-01-02"), to.Format("2006-01-02"))
			if err != nil {
				return nil, err
			}
			present, total := 0, 0
			var lastMark *time.Time
			for _, cs := range expand(scheds, from, to, now, marked) {
				switch cs.Status {
				case "present":
					present++
					total++
				case "absent":
					total++
				}
			}
			for _, at := range marked {
				a := at
				if lastMark == nil || a.After(*lastMark) {
					lastMark = &a
				}
			}
			touch(s, lastMark)
			if total > 0 {
				pct := present * 100 / total
				s.AttendancePct, s.AttendanceTotal = &pct, total
				s.LowAttendance = pct < lowAttendancePct
			}
		}
	}

	cut := now.AddDate(0, 0, -inactiveAfterDays)
	for _, s := range out {
		s.Inactive = s.LastActiveAt == nil || s.LastActiveAt.Before(cut)
	}
	return out, nil
}

// GET /api/college/overview
func (h *CollegeHandler) overview(w http.ResponseWriter, r *http.Request, viewerID, collegeID, name, code string) {
	ctx := r.Context()
	students, err := h.loadStudents(ctx, collegeID)
	if err != nil {
		h.Log.Error("college overview failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not load your students")
		return
	}
	now := h.now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, classTZ)
	weekAgo := now.AddDate(0, 0, -7)
	var joinedThisMonth, active7, certified, inactive, lowAtt, flagged, progressSum int
	for _, s := range students {
		if s.LinkedAt != nil && !s.LinkedAt.Before(monthStart) {
			joinedThisMonth++
		}
		if s.LastActiveAt != nil && s.LastActiveAt.After(weekAgo) {
			active7++
		}
		if s.Certificates > 0 {
			certified++
		}
		if s.Inactive {
			inactive++
		}
		if s.LowAttendance {
			lowAtt++
		}
		if s.IntegrityFlag {
			flagged++
		}
		progressSum += s.CoursePercent
	}
	avgProgress := 0
	if len(students) > 0 {
		avgProgress = progressSum / len(students)
	}
	// Certification exam pass rate across the college's students.
	var passed, failed int
	if len(students) > 0 {
		ids := make([]string, 0, len(students))
		for _, s := range students {
			ids = append(ids, s.UserID)
		}
		if err := h.Pool.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE status = 'passed'), count(*) FILTER (WHERE status = 'failed')
			FROM   certification_registrations WHERE user_id = ANY($1::uuid[])
		`, ids).Scan(&passed, &failed); err != nil {
			h.Log.Warn("college pass rate failed", zap.Error(err))
		}
	}
	var passRate *int
	if passed+failed > 0 {
		p := passed * 100 / (passed + failed)
		passRate = &p
	}
	var pending int
	_ = h.Pool.QueryRow(ctx, `SELECT count(*) FROM college_students WHERE college_id = $1::uuid AND status = 'pending'`,
		collegeID).Scan(&pending)

	h.logAccess(ctx, collegeID, viewerID, "overview", fmt.Sprintf("%d students", len(students)))
	writeJSON(w, http.StatusOK, map[string]any{
		"college":           map[string]any{"id": collegeID, "name": name, "joinCode": code},
		"linked":            len(students),
		"joinedThisMonth":   joinedThisMonth,
		"pendingRequests":   pending,
		"active7d":          active7,
		"avgProgress":       avgProgress,
		"certified":         certified,
		"certPassRate":      passRate,
		"needsAttention":    map[string]int{"inactive": inactive, "lowAttendance": lowAtt, "flagged": flagged},
		"inactiveAfterDays": inactiveAfterDays,
		"lowAttendancePct":  lowAttendancePct,
	})
}

// GET /api/college/students?q=&branch=&year=&filter=inactive|attendance|flagged&format=csv
func (h *CollegeHandler) students(w http.ResponseWriter, r *http.Request, viewerID, collegeID, collegeName string) {
	ctx := r.Context()
	all, err := h.loadStudents(ctx, collegeID)
	if err != nil {
		h.Log.Error("college students failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not load your students")
		return
	}
	q := r.URL.Query()
	needle := strings.ToLower(strings.TrimSpace(q.Get("q")))
	branch, year, filter := q.Get("branch"), q.Get("year"), q.Get("filter")
	branches, years := map[string]bool{}, map[string]bool{}
	rows := []*collegeStudentRow{}
	for _, s := range all {
		if s.Branch != "" {
			branches[s.Branch] = true
		}
		if s.Year != "" {
			years[s.Year] = true
		}
		if branch != "" && s.Branch != branch || year != "" && s.Year != year {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(s.Name+" "+s.Email+" "+s.RollNo), needle) {
			continue
		}
		switch filter {
		case "inactive":
			if !s.Inactive {
				continue
			}
		case "attendance":
			if !s.LowAttendance {
				continue
			}
		case "flagged":
			if !s.IntegrityFlag {
				continue
			}
		}
		rows = append(rows, s)
	}

	if q.Get("format") == "csv" {
		h.logAccess(ctx, collegeID, viewerID, "export", fmt.Sprintf("%d of %d students", len(rows), len(all)))
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+csvFileName(collegeName)+`"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"Name", "Email", "Roll no", "Branch", "Year", "Course", "Course progress %",
			"Attendance % (30 days)", "Latest test", "Latest test %", "Integrity flag", "Certificates", "Last active"})
		for _, s := range rows {
			att, test, testPct, last := "", "", "", ""
			if s.AttendancePct != nil {
				att = strconv.Itoa(*s.AttendancePct)
			}
			if s.LatestTest != nil {
				test, testPct = s.LatestTest.Title, strconv.FormatFloat(s.LatestTest.Percent, 'f', 1, 64)
			}
			if s.LastActiveAt != nil {
				last = s.LastActiveAt.In(classTZ).Format("2006-01-02")
			}
			flag := ""
			if s.IntegrityFlag {
				flag = "yes"
			}
			_ = cw.Write(csvSafeRow([]string{s.Name, s.Email, s.RollNo, s.Branch, s.Year, s.CourseTitle,
				strconv.Itoa(s.CoursePercent), att, test, testPct, flag, strconv.Itoa(s.Certificates), last}))
		}
		cw.Flush()
		return
	}

	h.logAccess(ctx, collegeID, viewerID, "list", fmt.Sprintf("%d of %d students", len(rows), len(all)))
	writeJSON(w, http.StatusOK, map[string]any{
		"students": rows,
		"total":    len(all),
		"branches": sortedKeys(branches),
		"years":    sortedKeys(years),
	})
}

// POST /api/college/roster {rows:[{email, rollNo, branch, year}]}
//
// Creates pending requests only — a student is never linked without saying
// yes. Emails that are not Knovate student accounts are reported back, not
// created: this view does not make accounts.
func (h *CollegeHandler) roster(w http.ResponseWriter, r *http.Request, viewerID, collegeID string) {
	var body struct {
		Rows []struct {
			Email string `json:"email"`
			linkDetails
		} `json:"rows"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Rows) == 0 {
		writeErr(w, http.StatusBadRequest, "the list is empty")
		return
	}
	if len(body.Rows) > maxRosterRows {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("upload at most %d students at a time", maxRosterRows))
		return
	}
	ctx := r.Context()
	requested, updated, alreadyLinked, otherCollege := 0, 0, 0, 0
	notFound := []string{}
	seen := map[string]bool{}
	for _, row := range body.Rows {
		email := strings.ToLower(clip(row.Email, 200))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		row.clean()
		var userID, role string
		var status, elsewhere *string
		err := h.Pool.QueryRow(ctx, `
			SELECT u.id::text, u.role,
			       (SELECT s.status FROM college_students s WHERE s.user_id = u.id AND s.college_id = $2::uuid),
			       (SELECT c.name FROM college_students s JOIN colleges c ON c.id = s.college_id
			         WHERE s.user_id = u.id AND s.status = 'linked' AND s.college_id <> $2::uuid)
			FROM   users u WHERE lower(u.email) = $1
		`, email, collegeID).Scan(&userID, &role, &status, &elsewhere)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "student") {
			notFound = append(notFound, email)
			continue
		}
		if err != nil {
			h.Log.Error("roster lookup failed", zap.Error(err))
			writeErr(w, http.StatusInternalServerError, "could not process the list, please try again")
			return
		}
		if elsewhere != nil {
			otherCollege++
			continue
		}
		if _, err := h.Pool.Exec(ctx, `
			INSERT INTO college_students (college_id, user_id, roll_no, branch, year, status, invited_at)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'pending', now())
			ON CONFLICT (college_id, user_id) DO UPDATE
			   SET roll_no = COALESCE(NULLIF(EXCLUDED.roll_no, ''), college_students.roll_no),
			       branch  = COALESCE(NULLIF(EXCLUDED.branch, ''),  college_students.branch),
			       year    = COALESCE(NULLIF(EXCLUDED.year, ''),    college_students.year)
		`, collegeID, userID, row.RollNo, row.Branch, row.Year); err != nil {
			h.Log.Error("roster write failed", zap.Error(err))
			writeErr(w, http.StatusInternalServerError, "could not process the list, please try again")
			return
		}
		switch {
		case status == nil:
			requested++
		case *status == "linked":
			alreadyLinked++
		default:
			updated++
		}
	}
	h.logAccess(ctx, collegeID, viewerID, "roster", fmt.Sprintf("%d rows, %d new requests", len(body.Rows), requested))
	sort.Strings(notFound)
	writeJSON(w, http.StatusOK, map[string]any{
		"requested":     requested,
		"updated":       updated,
		"alreadyLinked": alreadyLinked,
		"otherCollege":  otherCollege,
		"notFound":      notFound,
	})
}

// ─── Admin: /api/college/admin/* ──────────────────────────────────────────────

func (h *CollegeHandler) admin(w http.ResponseWriter, r *http.Request, adminID string, parts []string) {
	ctx := r.Context()
	switch {
	case len(parts) == 1 && parts[0] == "colleges" && r.Method == http.MethodGet:
		rows, err := h.Pool.Query(ctx, `
			SELECT c.id::text, c.name, c.city, c.join_code, c.created_at,
			       (SELECT count(*) FROM college_students s WHERE s.college_id = c.id AND s.status = 'linked'),
			       (SELECT count(*) FROM college_students s WHERE s.college_id = c.id AND s.status = 'pending'),
			       COALESCE((SELECT json_agg(json_build_object('userId', u.id, 'name', u.name, 'email', u.email) ORDER BY u.name)
			                 FROM college_staff st JOIN users u ON u.id = st.user_id WHERE st.college_id = c.id), '[]')
			FROM   colleges c ORDER BY c.name
		`)
		if err != nil {
			h.Log.Error("list colleges failed", zap.Error(err))
			writeErr(w, http.StatusInternalServerError, "could not load colleges")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, name, city, code string
			var created time.Time
			var linked, pending int
			var staff json.RawMessage
			if err := rows.Scan(&id, &name, &city, &code, &created, &linked, &pending, &staff); err != nil {
				h.Log.Error("scan college failed", zap.Error(err))
				writeErr(w, http.StatusInternalServerError, "could not load colleges")
				return
			}
			out = append(out, map[string]any{"id": id, "name": name, "city": city, "joinCode": code,
				"createdAt": created, "linked": linked, "pending": pending, "staff": staff})
		}
		writeJSON(w, http.StatusOK, map[string]any{"colleges": out})

	case len(parts) == 1 && parts[0] == "colleges" && r.Method == http.MethodPost:
		var body struct {
			Name string `json:"name"`
			City string `json:"city"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		name, city := clip(body.Name, 160), clip(body.City, 80)
		if name == "" {
			writeErr(w, http.StatusBadRequest, "the college needs a name")
			return
		}
		var id, code string
		for try := 0; try < 5; try++ {
			c, err := newJoinCode()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "could not create the college")
				return
			}
			err = h.Pool.QueryRow(ctx, `
				INSERT INTO colleges (name, city, join_code) VALUES ($1, $2, $3)
				ON CONFLICT (join_code) DO NOTHING RETURNING id::text, join_code
			`, name, city, c).Scan(&id, &code)
			if err == nil {
				break
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				h.Log.Error("create college failed", zap.Error(err))
				writeErr(w, http.StatusInternalServerError, "could not create the college")
				return
			}
		}
		if id == "" {
			writeErr(w, http.StatusInternalServerError, "could not create the college")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name, "city": city, "joinCode": code})

	case len(parts) == 3 && parts[0] == "colleges" && parts[2] == "code" && r.Method == http.MethodPost:
		// A leaked code: issue a new one. Students already linked stay linked.
		for try := 0; try < 5; try++ {
			c, err := newJoinCode()
			if err != nil {
				break
			}
			var code string
			err = h.Pool.QueryRow(ctx, `UPDATE colleges SET join_code = $2 WHERE id = $1::uuid RETURNING join_code`,
				parts[1], c).Scan(&code)
			if err == nil {
				writeJSON(w, http.StatusOK, map[string]any{"joinCode": code})
				return
			}
			if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "invalid input syntax") {
				writeErr(w, http.StatusNotFound, "college not found")
				return
			}
		}
		writeErr(w, http.StatusInternalServerError, "could not issue a new code")

	case len(parts) == 3 && parts[0] == "colleges" && parts[2] == "staff" && r.Method == http.MethodPost:
		h.addStaff(w, r, parts[1])

	case len(parts) == 4 && parts[0] == "colleges" && parts[2] == "staff" && r.Method == http.MethodDelete:
		tx, err := h.Pool.Begin(ctx)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not remove staff")
			return
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		tag, err := tx.Exec(ctx, `DELETE FROM college_staff WHERE college_id = $1::uuid AND user_id = $2::uuid`, parts[1], parts[3])
		if err != nil || tag.RowsAffected() == 0 {
			writeErr(w, http.StatusNotFound, "not a staff member of this college")
			return
		}
		// No college left: back to an ordinary student account.
		if _, err := tx.Exec(ctx, `
			UPDATE users SET role = 'student', updated_at = now()
			WHERE id = $1::uuid AND role = $2 AND NOT EXISTS (SELECT 1 FROM college_staff WHERE user_id = $1::uuid)
		`, parts[3], collegeRoleStaff); err != nil {
			writeErr(w, http.StatusInternalServerError, "could not remove staff")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeErr(w, http.StatusInternalServerError, "could not remove staff")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"removed": true})

	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

// addStaff attaches an existing account to a college as placement-cell staff.
// Only student or already-staff accounts can be made staff: an admin or a
// recruiter is never silently re-roled.
func (h *CollegeHandler) addStaff(w http.ResponseWriter, r *http.Request, collegeID string) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.ToLower(clip(body.Email, 200))
	ctx := r.Context()
	var userID, role, name string
	err := h.Pool.QueryRow(ctx, `SELECT id::text, role, name FROM users WHERE lower(email) = $1`, email).Scan(&userID, &role, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "no Knovate account uses that email — create the user first, then add them here")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not add staff")
		return
	}
	if role != "student" && role != collegeRoleStaff {
		writeErr(w, http.StatusConflict, "that account has the "+role+" role — use a separate account for college staff")
		return
	}
	var other string
	if err := h.Pool.QueryRow(ctx, `
		SELECT c.name FROM college_staff s JOIN colleges c ON c.id = s.college_id
		WHERE s.user_id = $1::uuid AND s.college_id <> $2::uuid
	`, userID, collegeID).Scan(&other); err == nil {
		writeErr(w, http.StatusConflict, "that account is already staff at "+other)
		return
	}
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not add staff")
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO college_staff (college_id, user_id) VALUES ($1::uuid, $2::uuid) ON CONFLICT DO NOTHING
	`, collegeID, userID); err != nil {
		if strings.Contains(err.Error(), "foreign key") || strings.Contains(err.Error(), "invalid input syntax") {
			writeErr(w, http.StatusNotFound, "college not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not add staff")
		return
	}
	// Staff see other students' records, so they must not also be a student
	// linked to a college themselves.
	if _, err := tx.Exec(ctx, `DELETE FROM college_students WHERE user_id = $1::uuid`, userID); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not add staff")
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET role = $2, updated_at = now() WHERE id = $1::uuid`, userID, collegeRoleStaff); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not add staff")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not add staff")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"userId": userID, "name": name, "email": email,
		"note": "they must sign out and back in for the new access to apply"})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// joinCodeAlphabet leaves out 0/O, 1/I/L so a code read off a projector is
// typed correctly.
const joinCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// newJoinCode returns "ABCD-EFGH": 8 characters, about 10^12 combinations.
func newJoinCode() (string, error) {
	b := make([]byte, 0, 9)
	for i := 0; i < 8; i++ {
		if i == 4 {
			b = append(b, '-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(joinCodeAlphabet))))
		if err != nil {
			return "", err
		}
		b = append(b, joinCodeAlphabet[n.Int64()])
	}
	return string(b), nil
}

// normaliseJoinCode accepts "abcd efgh", "ABCDEFGH" or "ABCD-EFGH".
func normaliseJoinCode(s string) string {
	var out []byte
	for _, c := range strings.ToUpper(s) {
		if strings.ContainsRune(joinCodeAlphabet, c) {
			out = append(out, byte(c))
		}
	}
	if len(out) != 8 {
		return ""
	}
	return string(out[:4]) + "-" + string(out[4:])
}

// csvSafeRow applies csvSafe to every cell: a student can set their own name
// to "=HYPERLINK(...)", and the placement officer opens this file in Excel.
func csvSafeRow(cells []string) []string {
	for i, c := range cells {
		cells[i] = csvSafe(c)
	}
	return cells
}

func csvFileName(college string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(college) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "college"
	}
	return name + "-students.csv"
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
