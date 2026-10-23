package resolvers

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// CourseProgressHandler stores how far a learner has read through each course.
//
// The course viewer used to keep this only in the browser's localStorage, so a
// learner on a second device started at 0% and the next learner on a shared
// browser inherited the previous one's progress. The browser now keeps a cache
// and this table is the record.
//
// A course is keyed by the viewer's storage key ("maxJavaIndexRead"): the
// lesson indexes are positions in that viewer's syllabus, which only the
// frontend knows, so the server stores them without interpreting them.
type CourseProgressHandler struct {
	Pool *pgxpool.Pool
	Log  *zap.Logger
}

// courseKeyPattern bounds what a client may use as a key. The viewers use
// camel-case names like "maxSqlIndexRead".
var courseKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,63}$`)

const (
	maxLessonIndex  = 10000
	maxLessonIDSize = 200
)

// NewCourseProgressHandler wires the handler and ensures its table exists.
func NewCourseProgressHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) (*CourseProgressHandler, error) {
	h := &CourseProgressHandler{Pool: pool, Log: log}
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS course_progress (
			user_id     TEXT NOT NULL,
			course_key  TEXT NOT NULL,
			max_index   INTEGER NOT NULL DEFAULT 0,
			last_lesson TEXT NOT NULL DEFAULT '',
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, course_key)
		)`)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (h *CourseProgressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/course-progress"), "/")
	switch {
	case path == "" && r.Method == http.MethodGet:
		h.list(w, r, userID)
	case path != "" && r.Method == http.MethodPut:
		h.save(w, r, userID, path)
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

type courseProgressRow struct {
	CourseKey  string `json:"course_key"`
	MaxIndex   int    `json:"max_index"`
	LastLesson string `json:"last_lesson"`
}

// GET /api/course-progress — every course the learner has opened.
func (h *CourseProgressHandler) list(w http.ResponseWriter, r *http.Request, userID string) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT course_key, max_index, last_lesson FROM course_progress WHERE user_id = $1`, userID)
	if err != nil {
		h.Log.Error("list course progress failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not load progress")
		return
	}
	defer rows.Close()

	out := []courseProgressRow{}
	for rows.Next() {
		var p courseProgressRow
		if err := rows.Scan(&p.CourseKey, &p.MaxIndex, &p.LastLesson); err != nil {
			h.Log.Error("scan course progress failed", zap.Error(err))
			writeErr(w, http.StatusInternalServerError, "could not load progress")
			return
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"progress": out})
}

// PUT /api/course-progress/{courseKey} {max_index, last_lesson}
//
// max_index only ever grows, so a stale tab or a second device can't move a
// learner backwards. last_lesson is where they were most recently and simply
// follows the latest write; an empty value leaves it unchanged.
func (h *CourseProgressHandler) save(w http.ResponseWriter, r *http.Request, userID, courseKey string) {
	if !courseKeyPattern.MatchString(courseKey) {
		writeErr(w, http.StatusBadRequest, "invalid course key")
		return
	}
	var body struct {
		MaxIndex   int    `json:"max_index"`
		LastLesson string `json:"last_lesson"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.MaxIndex < 0 || body.MaxIndex > maxLessonIndex || len(body.LastLesson) > maxLessonIDSize {
		writeErr(w, http.StatusBadRequest, "progress out of range")
		return
	}

	var saved courseProgressRow
	err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO course_progress (user_id, course_key, max_index, last_lesson)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, course_key) DO UPDATE SET
			max_index   = GREATEST(course_progress.max_index, EXCLUDED.max_index),
			last_lesson = COALESCE(NULLIF(EXCLUDED.last_lesson, ''), course_progress.last_lesson),
			updated_at  = now()
		RETURNING course_key, max_index, last_lesson`,
		userID, courseKey, body.MaxIndex, body.LastLesson,
	).Scan(&saved.CourseKey, &saved.MaxIndex, &saved.LastLesson)
	if err != nil {
		h.Log.Error("save course progress failed", zap.Error(err))
		writeErr(w, http.StatusInternalServerError, "could not save progress")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
