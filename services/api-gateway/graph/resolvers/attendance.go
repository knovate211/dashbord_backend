package resolvers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	_ "time/tzdata" // the runtime image may ship without a zoneinfo database

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// AttendanceHandler owns live-class schedules and the attendance students mark
// against them.
//
// A schedule is a weekly recurrence ("Java, Mon/Wed/Fri 11:00–12:00, from
// 1 Sep to 30 Nov"). Individual sessions are never stored — they are expanded
// from the schedule on read. Only a "present" is written; "absent" is a session
// that has ended with no row, so nothing needs a sweeper to close the day.
//
// Who is expected at a class is decided by user_courses: every student granted
// the schedule's course. The attendance window is the class itself — a student
// can mark from start time until end time, and the server clock decides, never
// the browser's.
//
// All times are wall-clock times in classTZ. The institute teaches in one
// timezone; storing a zone per schedule would invite the admin screen and the
// student screen to disagree about when 11:00 is.
type AttendanceHandler struct {
	Pool *pgxpool.Pool
	Log  *zap.Logger
	// now is overridable so tests can stand at a fixed moment.
	now func() time.Time
}

var classTZ = mustLoadLocation("Asia/Kolkata")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// NewAttendanceHandler wires the handler and ensures its tables exist.
func NewAttendanceHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) (*AttendanceHandler, error) {
	h := &AttendanceHandler{Pool: pool, Log: log, now: time.Now}
	if err := h.ensureTables(ctx); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *AttendanceHandler) ensureTables(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS class_schedules (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			course_id    TEXT NOT NULL,
			title        TEXT NOT NULL,
			instructor   TEXT NOT NULL DEFAULT '',
			meeting_url  TEXT NOT NULL DEFAULT '',
			-- 0 = Sunday … 6 = Saturday, matching Go's time.Weekday.
			days_of_week SMALLINT[] NOT NULL,
			start_time   TIME NOT NULL,
			end_time     TIME NOT NULL,
			start_date   DATE NOT NULL,
			end_date     DATE NOT NULL,
			is_active    BOOLEAN NOT NULL DEFAULT true,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			CHECK (end_time > start_time),
			CHECK (end_date >= start_date)
		);
		CREATE INDEX IF NOT EXISTS idx_class_schedules_course ON class_schedules(course_id);

		CREATE TABLE IF NOT EXISTS class_attendance (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			schedule_id UUID NOT NULL REFERENCES class_schedules(id) ON DELETE CASCADE,
			user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			class_date  DATE NOT NULL,
			marked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (schedule_id, user_id, class_date)
		);
		CREATE INDEX IF NOT EXISTS idx_class_attendance_user ON class_attendance(user_id, class_date);
	`)
	if err != nil {
		return fmt.Errorf("create attendance tables: %w", err)
	}
	return nil
}

// ─── Schedule model ───────────────────────────────────────────────────────────

type classSchedule struct {
	ID         string `json:"id"`
	CourseID   string `json:"course_id"`
	Title      string `json:"title"`
	Instructor string `json:"instructor"`
	MeetingURL string `json:"meeting_url"`
	Days       []int  `json:"days_of_week"`
	StartTime  string `json:"start_time"` // HH:MM
	EndTime    string `json:"end_time"`   // HH:MM
	StartDate  string `json:"start_date"` // YYYY-MM-DD
	EndDate    string `json:"end_date"`   // YYYY-MM-DD
	IsActive   bool   `json:"is_active"`
	// joinedOn is the student's enrolment date (YYYY-MM-DD, class time). Sessions
	// before it are not theirs to have missed. Empty outside student reads.
	joinedOn string
}

// runsOn reports whether the schedule has a session on the given calendar date.
func (s classSchedule) runsOn(date string, weekday time.Weekday) bool {
	if date < s.StartDate || date > s.EndDate || date < s.joinedOn {
		return false
	}
	for _, d := range s.Days {
		if d == int(weekday) {
			return true
		}
	}
	return false
}

// window returns the session's start and end instants on the given date.
func (s classSchedule) window(date time.Time) (time.Time, time.Time) {
	at := func(hhmm string) time.Time {
		t, _ := time.Parse("15:04", hhmm)
		return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), 0, 0, classTZ)
	}
	return at(s.StartTime), at(s.EndTime)
}

// validate normalises and checks an admin-supplied schedule.
func (s *classSchedule) validate() error {
	s.CourseID = clip(s.CourseID, 60)
	s.Title = clip(s.Title, 160)
	s.Instructor = clip(s.Instructor, 120)
	s.MeetingURL = clip(s.MeetingURL, 500)
	if s.CourseID == "" || s.Title == "" {
		return errors.New("course and title are required")
	}
	if s.MeetingURL != "" && !strings.HasPrefix(s.MeetingURL, "https://") && !strings.HasPrefix(s.MeetingURL, "http://") {
		return errors.New("meeting link must start with http:// or https://")
	}

	seen := map[int]bool{}
	days := []int{}
	for _, d := range s.Days {
		if d < 0 || d > 6 {
			return errors.New("days_of_week must be between 0 (Sunday) and 6 (Saturday)")
		}
		if !seen[d] {
			seen[d] = true
			days = append(days, d)
		}
	}
	if len(days) == 0 {
		return errors.New("choose at least one day of the week")
	}
	sort.Ints(days)
	s.Days = days

	st, err1 := time.Parse("15:04", s.StartTime)
	et, err2 := time.Parse("15:04", s.EndTime)
	if err1 != nil || err2 != nil {
		return errors.New("start and end time must be HH:MM")
	}
	if !et.After(st) {
		return errors.New("the class must end after it starts")
	}

	sd, err1 := time.Parse("2006-01-02", s.StartDate)
	ed, err2 := time.Parse("2006-01-02", s.EndDate)
	if err1 != nil || err2 != nil {
		return errors.New("start and end date must be YYYY-MM-DD")
	}
	if ed.Before(sd) {
		return errors.New("the end date is before the start date")
	}
	return nil
}

// scheduleColumns expects class_schedules to be aliased as c.
const scheduleColumns = `c.id::text, c.course_id, c.title, c.instructor, c.meeting_url, c.days_of_week,
	to_char(c.start_time, 'HH24:MI'), to_char(c.end_time, 'HH24:MI'),
	to_char(c.start_date, 'YYYY-MM-DD'), to_char(c.end_date, 'YYYY-MM-DD'), c.is_active`

func scanSchedule(row pgx.Row) (classSchedule, error) {
	var s classSchedule
	var days []int16
	err := row.Scan(&s.ID, &s.CourseID, &s.Title, &s.Instructor, &s.MeetingURL, &days,
		&s.StartTime, &s.EndTime, &s.StartDate, &s.EndDate, &s.IsActive)
	for _, d := range days {
		s.Days = append(s.Days, int(d))
	}
	return s, err
}

// ─── Student routes: /api/attendance/* ────────────────────────────────────────

// ServeHTTP dispatches the authenticated student routes.
func (h *AttendanceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		h.fail(w, http.StatusUnauthorized, "authentication required")
		return
	}

	switch path := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/attendance"), "/"); {
	case path == "/today" && r.Method == http.MethodGet:
		h.today(w, r, userID)
	case path == "/mark" && r.Method == http.MethodPost:
		h.mark(w, r, userID)
	case path == "/history" && r.Method == http.MethodGet:
		h.history(w, r, userID)
	default:
		h.fail(w, http.StatusNotFound, "not found")
	}
}

type classSession struct {
	ScheduleID string `json:"schedule_id"`
	CourseID   string `json:"course_id"`
	Title      string `json:"title"`
	Instructor string `json:"instructor"`
	MeetingURL string `json:"meeting_url"`
	Date       string `json:"date"`
	StartsAt   string `json:"starts_at"`
	EndsAt     string `json:"ends_at"`
	// upcoming | live | present | absent. "live" means open and not yet marked.
	Status   string `json:"status"`
	MarkedAt string `json:"marked_at,omitempty"`
}

// studentSchedules loads the active schedules for every course the student
// holds, each stamped with the day the student was granted that course.
func (h *AttendanceHandler) studentSchedules(ctx context.Context, userID string) ([]classSchedule, error) {
	rows, err := h.Pool.Query(ctx, `
		SELECT `+scheduleColumns+`,
		       to_char((uc.granted_at AT TIME ZONE 'Asia/Kolkata')::date, 'YYYY-MM-DD')
		FROM   class_schedules c
		JOIN   user_courses uc ON uc.course_id = c.course_id AND uc.user_id = $1::uuid
		WHERE  c.is_active
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []classSchedule
	for rows.Next() {
		var s classSchedule
		var days []int16
		if err := rows.Scan(&s.ID, &s.CourseID, &s.Title, &s.Instructor, &s.MeetingURL, &days,
			&s.StartTime, &s.EndTime, &s.StartDate, &s.EndDate, &s.IsActive, &s.joinedOn); err != nil {
			return nil, err
		}
		for _, d := range days {
			s.Days = append(s.Days, int(d))
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// markedIn returns marked_at keyed by "scheduleID|date" for a date range.
func (h *AttendanceHandler) markedIn(ctx context.Context, userID, from, to string) (map[string]time.Time, error) {
	rows, err := h.Pool.Query(ctx, `
		SELECT schedule_id::text, to_char(class_date, 'YYYY-MM-DD'), marked_at
		FROM   class_attendance
		WHERE  user_id = $1::uuid AND class_date BETWEEN $2::date AND $3::date
	`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var sid, date string
		var at time.Time
		if err := rows.Scan(&sid, &date, &at); err != nil {
			return nil, err
		}
		out[sid+"|"+date] = at
	}
	return out, rows.Err()
}

// expand turns schedules into dated sessions over [from, to], newest first.
func expand(schedules []classSchedule, from, to time.Time, now time.Time, marked map[string]time.Time) []classSession {
	var out []classSession
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		for _, s := range schedules {
			if !s.runsOn(date, d.Weekday()) {
				continue
			}
			start, end := s.window(d)
			cs := classSession{
				ScheduleID: s.ID, CourseID: s.CourseID, Title: s.Title,
				Instructor: s.Instructor, MeetingURL: s.MeetingURL, Date: date,
				StartsAt: start.Format(time.RFC3339), EndsAt: end.Format(time.RFC3339),
			}
			at, ok := marked[s.ID+"|"+date]
			switch {
			case ok:
				cs.Status, cs.MarkedAt = "present", at.In(classTZ).Format(time.RFC3339)
			case now.Before(start):
				cs.Status = "upcoming"
			case now.Before(end):
				cs.Status = "live"
			default:
				cs.Status = "absent"
			}
			out = append(out, cs)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt > out[j].StartsAt })
	return out
}

func dayStart(t time.Time) time.Time {
	t = t.In(classTZ)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, classTZ)
}

// today backs GET /api/attendance/today — every session today, for the pop-up.
// The response carries server_time so the client can correct a skewed clock.
func (h *AttendanceHandler) today(w http.ResponseWriter, r *http.Request, userID string) {
	now := h.now()
	day := dayStart(now)
	date := day.Format("2006-01-02")

	schedules, err := h.studentSchedules(r.Context(), userID)
	if err != nil {
		h.Log.Error("load schedules failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load today's classes")
		return
	}
	marked, err := h.markedIn(r.Context(), userID, date, date)
	if err != nil {
		h.Log.Error("load attendance failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load today's classes")
		return
	}
	sessions := expand(schedules, day, day, now, marked)
	// The pop-up reads in time order.
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartsAt < sessions[j].StartsAt })
	h.json(w, http.StatusOK, map[string]any{
		"sessions":    nonNil(sessions),
		"server_time": now.In(classTZ).Format(time.RFC3339),
	})
}

// mark backs POST /api/attendance/mark. The session date is taken from the
// server clock, so a student can only ever mark the class running right now.
func (h *AttendanceHandler) mark(w http.ResponseWriter, r *http.Request, userID string) {
	var body struct {
		ScheduleID string `json:"schedule_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.ScheduleID == "" {
		h.fail(w, http.StatusBadRequest, "schedule_id is required")
		return
	}

	s, err := scanSchedule(h.Pool.QueryRow(r.Context(), `
		SELECT `+scheduleColumns+`
		FROM   class_schedules c
		WHERE  c.id = $1::uuid AND c.is_active
		  AND  EXISTS (SELECT 1 FROM user_courses uc WHERE uc.user_id = $2::uuid AND uc.course_id = c.course_id)
	`, body.ScheduleID, userID))
	if errors.Is(err, pgx.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "invalid input syntax")) {
		h.fail(w, http.StatusNotFound, "class not found")
		return
	}
	if err != nil {
		h.Log.Error("load schedule for mark failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not mark attendance")
		return
	}

	if code, msg := checkMarkWindow(s, h.now()); code != 0 {
		h.fail(w, code, msg)
		return
	}

	date := dayStart(h.now()).Format("2006-01-02")
	// ON CONFLICT keeps a double-click or a second tab from failing — the first
	// mark stands and both callers see its time.
	var markedAt time.Time
	err = h.Pool.QueryRow(r.Context(), `
		WITH ins AS (
			INSERT INTO class_attendance (schedule_id, user_id, class_date)
			VALUES ($1::uuid, $2::uuid, $3::date)
			ON CONFLICT (schedule_id, user_id, class_date) DO NOTHING
			RETURNING marked_at
		)
		SELECT marked_at FROM ins
		UNION ALL
		SELECT marked_at FROM class_attendance
		WHERE  schedule_id = $1::uuid AND user_id = $2::uuid AND class_date = $3::date
		LIMIT 1
	`, s.ID, userID, date).Scan(&markedAt)
	if err != nil {
		h.Log.Error("mark attendance failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not mark attendance")
		return
	}
	h.json(w, http.StatusOK, map[string]any{
		"success": true, "marked_at": markedAt.In(classTZ).Format(time.RFC3339),
	})
}

// checkMarkWindow decides whether a schedule accepts a mark at the given moment.
// It returns 0 when it does, otherwise the HTTP status and message to send.
func checkMarkWindow(s classSchedule, now time.Time) (int, string) {
	day := dayStart(now)
	if !s.runsOn(day.Format("2006-01-02"), day.Weekday()) {
		return http.StatusConflict, "this class is not scheduled today"
	}
	start, end := s.window(day)
	if now.Before(start) {
		return http.StatusConflict, "attendance opens when the class starts at " + start.Format("3:04 PM")
	}
	if !now.Before(end) {
		return http.StatusConflict, "attendance closed when the class ended at " + end.Format("3:04 PM")
	}
	return 0, ""
}

const maxHistoryDays = 180

// history backs GET /api/attendance/history?from=YYYY-MM-DD&to=YYYY-MM-DD.
// Defaults to the last 30 days through today; future dates are clamped away.
func (h *AttendanceHandler) history(w http.ResponseWriter, r *http.Request, userID string) {
	now := h.now()
	today := dayStart(now)
	to, from := today, today.AddDate(0, 0, -29)
	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, classTZ)
		if err != nil {
			h.fail(w, http.StatusBadRequest, "to must be YYYY-MM-DD")
			return
		}
		if t.Before(today) {
			to = t
		}
	}
	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, classTZ)
		if err != nil {
			h.fail(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
			return
		}
		from = t
	}
	if from.After(to) {
		h.fail(w, http.StatusBadRequest, "from is after to")
		return
	}
	if to.Sub(from) > maxHistoryDays*24*time.Hour {
		from = to.AddDate(0, 0, -maxHistoryDays)
	}

	schedules, err := h.studentSchedules(r.Context(), userID)
	if err != nil {
		h.Log.Error("load schedules failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load attendance")
		return
	}
	marked, err := h.markedIn(r.Context(), userID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		h.Log.Error("load attendance failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load attendance")
		return
	}
	sessions := expand(schedules, from, to, now, marked)

	type courseSummary struct {
		CourseID string `json:"course_id"`
		Title    string `json:"title"`
		Present  int    `json:"present"`
		Absent   int    `json:"absent"`
	}
	byCourse := map[string]*courseSummary{}
	var order []string
	present, absent := 0, 0
	for _, s := range sessions {
		if s.Status != "present" && s.Status != "absent" {
			continue // a class that has not finished yet counts for neither
		}
		cs, ok := byCourse[s.ScheduleID]
		if !ok {
			cs = &courseSummary{CourseID: s.CourseID, Title: s.Title}
			byCourse[s.ScheduleID] = cs
			order = append(order, s.ScheduleID)
		}
		if s.Status == "present" {
			cs.Present++
			present++
		} else {
			cs.Absent++
			absent++
		}
	}
	courses := []courseSummary{}
	for _, id := range order {
		courses = append(courses, *byCourse[id])
	}

	h.json(w, http.StatusOK, map[string]any{
		"from":     from.Format("2006-01-02"),
		"to":       to.Format("2006-01-02"),
		"present":  present,
		"absent":   absent,
		"courses":  courses,
		"sessions": nonNil(sessions),
	})
}

// ─── Admin routes: /api/admin/classes* ────────────────────────────────────────

type adminSchedule struct {
	classSchedule
	Enrolled int `json:"enrolled"`
}

// ListSchedules backs GET /api/admin/classes.
func (h *AttendanceHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT `+scheduleColumns+`,
		       (SELECT COUNT(*) FROM user_courses uc WHERE uc.course_id = c.course_id)
		FROM   class_schedules c
		ORDER  BY c.is_active DESC, c.start_date DESC, c.start_time
	`)
	if err != nil {
		h.Log.Error("list schedules failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load classes")
		return
	}
	defer rows.Close()
	out := []adminSchedule{}
	for rows.Next() {
		var a adminSchedule
		var days []int16
		if err := rows.Scan(&a.ID, &a.CourseID, &a.Title, &a.Instructor, &a.MeetingURL, &days,
			&a.StartTime, &a.EndTime, &a.StartDate, &a.EndDate, &a.IsActive, &a.Enrolled); err != nil {
			h.Log.Error("scan schedule failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load classes")
			return
		}
		for _, d := range days {
			a.Days = append(a.Days, int(d))
		}
		out = append(out, a)
	}
	h.json(w, http.StatusOK, map[string]any{"classes": out})
}

func (h *AttendanceHandler) decodeSchedule(w http.ResponseWriter, r *http.Request) (classSchedule, bool) {
	var s classSchedule
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&s); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return s, false
	}
	if err := s.validate(); err != nil {
		h.fail(w, http.StatusBadRequest, err.Error())
		return s, false
	}
	return s, true
}

// CreateSchedule backs POST /api/admin/classes.
func (h *AttendanceHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	s, ok := h.decodeSchedule(w, r)
	if !ok {
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO class_schedules
			(course_id, title, instructor, meeting_url, days_of_week, start_time, end_time, start_date, end_date, is_active)
		VALUES ($1, $2, $3, $4, $5, $6::time, $7::time, $8::date, $9::date, $10)
		RETURNING id::text
	`, s.CourseID, s.Title, s.Instructor, s.MeetingURL, s.Days, s.StartTime, s.EndTime,
		s.StartDate, s.EndDate, s.IsActive).Scan(&id)
	if err != nil {
		h.Log.Error("create schedule failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not create the class")
		return
	}
	h.json(w, http.StatusOK, map[string]any{"success": true, "id": id})
}

// UpdateSchedule backs PUT /api/admin/classes/{id}.
func (h *AttendanceHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request, id string) {
	s, ok := h.decodeSchedule(w, r)
	if !ok {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `
		UPDATE class_schedules
		SET    course_id = $2, title = $3, instructor = $4, meeting_url = $5, days_of_week = $6,
		       start_time = $7::time, end_time = $8::time, start_date = $9::date, end_date = $10::date,
		       is_active = $11, updated_at = now()
		WHERE  id = $1::uuid
	`, id, s.CourseID, s.Title, s.Instructor, s.MeetingURL, s.Days, s.StartTime, s.EndTime,
		s.StartDate, s.EndDate, s.IsActive)
	if err != nil {
		h.Log.Error("update schedule failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update the class")
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, http.StatusNotFound, "class not found")
		return
	}
	h.json(w, http.StatusOK, map[string]any{"success": true})
}

// DeleteSchedule backs DELETE /api/admin/classes/{id}. Its attendance goes with
// it; pausing (is_active=false) is the way to stop a class but keep the record.
func (h *AttendanceHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request, id string) {
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM class_schedules WHERE id = $1::uuid`, id)
	if err != nil {
		h.Log.Error("delete schedule failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not delete the class")
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, http.StatusNotFound, "class not found")
		return
	}
	h.json(w, http.StatusOK, map[string]any{"success": true})
}

// SessionRoster backs GET /api/admin/classes/{id}/attendance?date=YYYY-MM-DD —
// every enrolled student and whether they marked that session.
func (h *AttendanceHandler) SessionRoster(w http.ResponseWriter, r *http.Request, id string) {
	s, err := scanSchedule(h.Pool.QueryRow(r.Context(),
		`SELECT `+scheduleColumns+` FROM class_schedules c WHERE c.id = $1::uuid`, id))
	if err != nil {
		h.fail(w, http.StatusNotFound, "class not found")
		return
	}

	now := h.now()
	date := r.URL.Query().Get("date")
	if date == "" {
		date = dayStart(now).Format("2006-01-02")
	}
	day, err := time.ParseInLocation("2006-01-02", date, classTZ)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	scheduled := s.runsOn(date, day.Weekday())
	start, end := s.window(day)
	state := "ended"
	switch {
	case !scheduled:
		state = "not_scheduled"
	case now.Before(start):
		state = "upcoming"
	case now.Before(end):
		state = "live"
	}

	rows, err := h.Pool.Query(r.Context(), `
		SELECT u.id::text, u.name, u.email, a.marked_at
		FROM   user_courses uc
		JOIN   users u ON u.id = uc.user_id
		LEFT   JOIN class_attendance a
		       ON a.user_id = u.id AND a.schedule_id = $1::uuid AND a.class_date = $2::date
		WHERE  uc.course_id = $3
		  AND  (uc.granted_at AT TIME ZONE 'Asia/Kolkata')::date <= $2::date
		ORDER  BY a.marked_at IS NULL, u.name
	`, s.ID, date, s.CourseID)
	if err != nil {
		h.Log.Error("load roster failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load attendance")
		return
	}
	defer rows.Close()

	type rosterRow struct {
		UserID   string `json:"user_id"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Status   string `json:"status"` // present | absent | pending
		MarkedAt string `json:"marked_at,omitempty"`
	}
	out := []rosterRow{}
	present := 0
	for rows.Next() {
		var rr rosterRow
		var at *time.Time
		if err := rows.Scan(&rr.UserID, &rr.Name, &rr.Email, &at); err != nil {
			h.Log.Error("scan roster failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load attendance")
			return
		}
		switch {
		case at != nil:
			rr.Status, rr.MarkedAt = "present", at.In(classTZ).Format(time.RFC3339)
			present++
		case state == "ended":
			rr.Status = "absent"
		default:
			rr.Status = "pending"
		}
		out = append(out, rr)
	}
	h.json(w, http.StatusOK, map[string]any{
		"class": s, "date": date, "state": state, "present": present, "students": out,
	})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func nonNil(s []classSession) []classSession {
	if s == nil {
		return []classSession{}
	}
	return s
}

func (h *AttendanceHandler) json(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *AttendanceHandler) fail(w http.ResponseWriter, code int, msg string) {
	h.json(w, code, map[string]string{"error": msg})
}
