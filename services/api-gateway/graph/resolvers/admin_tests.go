package resolvers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// POST /api/admin/tests/{id}/duplicate  {title}
//
// Copies a test — settings, sections, and the questions attached to each
// section — as a new draft. The copy has its own id, so it gets its own
// attempts: a candidate who sat the original can still sit the copy. That is
// what makes it the right way to open a scholarship for another course: each
// course gets a paper titled for it, and one course's attempt does not use up
// another's.
//
// The copy starts as a draft so it can be reviewed and renamed before anyone
// can be invited to it; publishing goes through the normal publish endpoint,
// which validates the paper and computes its total marks.
func (h *AdminHandler) handleDuplicateTest(w http.ResponseWriter, r *http.Request, srcID string) {
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || len(req.Title) > 200 {
		h.jsonErr(w, http.StatusBadRequest, "title is required (at most 200 characters)")
		return
	}

	ctx := r.Context()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.jsonErr(w, http.StatusInternalServerError, "could not duplicate the test")
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	newID, sections, questions, err := duplicateTestTx(ctx, tx, srcID, req.Title, middleware.UserIDFromContext(ctx))
	if errors.Is(err, pgx.ErrNoRows) {
		h.jsonErr(w, http.StatusNotFound, "test not found")
		return
	}
	if err != nil {
		h.Log.Error("duplicate test failed", zap.String("src", srcID), zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "could not duplicate the test")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.Log.Error("commit duplicate test failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "could not duplicate the test")
		return
	}

	h.audit(ctx, "test.duplicated", "", "", map[string]any{
		"source_id": srcID, "new_id": newID, "title": req.Title,
		"sections": sections, "questions": questions,
	})
	h.json(w, http.StatusOK, map[string]any{"id": newID, "sections": sections, "questions": questions})
}

func duplicateTestTx(ctx context.Context, tx pgx.Tx, srcID, title, actorID string) (newID string, sections, questions int, err error) {
	// created_by is NOT NULL; the copy is credited to the admin who made it,
	// falling back to the original author if no actor id is on the request.
	if err = tx.QueryRow(ctx, `
		INSERT INTO assessments (
			company_id, title, description, purpose, duration_minutes, total_marks, passing_marks,
			negative_marking, shuffle_questions, shuffle_options, allow_backtrack, reveal_results,
			proctoring, status, opens_at, closes_at, max_attempts, created_by, lock_forward, course_ids)
		SELECT company_id, $2, description, purpose, duration_minutes, total_marks, passing_marks,
		       negative_marking, shuffle_questions, shuffle_options, allow_backtrack, reveal_results,
		       proctoring, 'draft', opens_at, closes_at, max_attempts,
		       COALESCE(NULLIF($3, '')::uuid, created_by), lock_forward, course_ids
		FROM   assessments WHERE id = $1::uuid
		RETURNING id::text
	`, srcID, title, actorID).Scan(&newID); err != nil {
		return "", 0, 0, err
	}

	rows, err := tx.Query(ctx, `SELECT id::text FROM assessment_sections WHERE assessment_id = $1::uuid ORDER BY order_index`, srcID)
	if err != nil {
		return "", 0, 0, err
	}
	var srcSections []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return "", 0, 0, err
		}
		srcSections = append(srcSections, id)
	}
	rows.Close()

	for _, oldSection := range srcSections {
		var newSection string
		if err = tx.QueryRow(ctx, `
			INSERT INTO assessment_sections (
				assessment_id, title, kind, order_index, duration_minutes, cutoff_marks,
				pick_count, pick_topic, pick_difficulty, pick_marks, partial_credit, pick_course)
			SELECT $2::uuid, title, kind, order_index, duration_minutes, cutoff_marks,
			       pick_count, pick_topic, pick_difficulty, pick_marks, partial_credit, pick_course
			FROM   assessment_sections WHERE id = $1::uuid
			RETURNING id::text
		`, oldSection, newID).Scan(&newSection); err != nil {
			return "", 0, 0, err
		}
		sections++

		tag, err := tx.Exec(ctx, `
			INSERT INTO section_questions (section_id, mcq_question_id, problem_id, marks, order_index)
			SELECT $2::uuid, mcq_question_id, problem_id, marks, order_index
			FROM   section_questions WHERE section_id = $1::uuid
		`, oldSection, newSection)
		if err != nil {
			return "", 0, 0, err
		}
		questions += int(tag.RowsAffected())
	}
	return newID, sections, questions, nil
}

// GET /api/admin/mcq-bank/facets — live question counts per course, topic and
// difficulty, for the course rail and filters on the question bank screen.
// Only active questions count; deleted ones are soft-deleted and hidden.
func (h *AdminHandler) handleMcqFacets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	course := r.URL.Query().Get("course")
	// Topic and difficulty counts follow the selected course, so the dropdowns
	// only offer values that exist inside it.
	cond, args := "is_active AND company_id IS NULL", []any{}
	switch course {
	case "":
	case "__general":
		cond += " AND course_id = ''"
	default:
		cond += " AND course_id = $1"
		args = append(args, course)
	}
	h.json(w, http.StatusOK, map[string]any{
		"course":     h.facetQuery(ctx, `SELECT course_id, '', COUNT(*)::int FROM mcq_questions WHERE is_active AND company_id IS NULL GROUP BY course_id ORDER BY 1`),
		"topic":      h.facetQuery(ctx, `SELECT topic, '', COUNT(*)::int FROM mcq_questions WHERE `+cond+` GROUP BY topic ORDER BY 3 DESC`, args...),
		"difficulty": h.facetQuery(ctx, `SELECT difficulty, '', COUNT(*)::int FROM mcq_questions WHERE `+cond+` GROUP BY difficulty ORDER BY 1`, args...),
	})
}
