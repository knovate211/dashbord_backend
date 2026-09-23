package repository

import (
	"context"
	"fmt"
	"strings"

	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// UpsertMcqQuestion creates or replaces a bank question together with its
// options. Options are rewritten wholesale inside the transaction, so a saved
// question can never end up with a stale answer key.
//
// Editing a question does not affect papers already in flight — attempts freeze
// their own copy of the option order at start and grade against the option ids
// they captured.
func (r *Repo) UpsertMcqQuestion(ctx context.Context, req *assessmentv1.UpsertMcqQuestionRequest) (string, error) {
	q := req.Question
	if q == nil || strings.TrimSpace(q.Body) == "" {
		return "", fmt.Errorf("question body is required")
	}
	if err := validateOptions(q); err != nil {
		return "", err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin upsert mcq: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	id := q.Id
	if id == "" {
		err = tx.QueryRow(ctx, `
			INSERT INTO mcq_questions (company_id, topic, difficulty, body, kind, explanation, is_active, created_by, course_id)
			VALUES ($1, $2, $3, $4, $5, $6, true, $7, $8)
			RETURNING id
		`, nullable(q.CompanyId), defaultStr(q.Topic, "General"), defaultStr(q.Difficulty, "Medium"),
			q.Body, defaultStr(q.Kind, "single"), q.Explanation, nullable(req.ActorId),
			strings.TrimSpace(q.CourseId)).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("insert mcq question: %w", err)
		}
	} else {
		ct, err := tx.Exec(ctx, `
			UPDATE mcq_questions
			SET    topic = $2, difficulty = $3, body = $4, kind = $5,
			       explanation = $6, is_active = $7, course_id = $8
			WHERE  id = $1
		`, id, defaultStr(q.Topic, "General"), defaultStr(q.Difficulty, "Medium"),
			q.Body, defaultStr(q.Kind, "single"), q.Explanation, q.IsActive,
			strings.TrimSpace(q.CourseId))
		if err != nil {
			return "", fmt.Errorf("update mcq question: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return "", ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM mcq_options WHERE question_id = $1`, id); err != nil {
			return "", fmt.Errorf("clear mcq options: %w", err)
		}
	}

	for i, opt := range q.Options {
		if _, err := tx.Exec(ctx, `
			INSERT INTO mcq_options (question_id, body, is_correct, order_index)
			VALUES ($1, $2, $3, $4)
		`, id, opt.Body, opt.IsCorrect, i); err != nil {
			return "", fmt.Errorf("insert mcq option: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit upsert mcq: %w", err)
	}
	return id, nil
}

// validateOptions rejects question shapes that could never be graded, rather
// than letting them reach a live paper.
func validateOptions(q *assessmentv1.McqQuestion) error {
	kind := defaultStr(q.Kind, "single")
	if kind == "numeric" {
		if len(q.Options) == 0 {
			return fmt.Errorf("a numeric question needs at least one accepted answer")
		}
		return nil
	}
	if len(q.Options) < 2 {
		return fmt.Errorf("a choice question needs at least two options")
	}
	correct := 0
	for _, o := range q.Options {
		if o.IsCorrect {
			correct++
		}
	}
	if correct == 0 {
		return fmt.Errorf("mark at least one option correct")
	}
	if kind == "single" && correct > 1 {
		return fmt.Errorf("a single-answer question cannot have %d correct options", correct)
	}
	return nil
}

// ListMcqQuestions returns bank questions with their options and answer key.
// This path is authoring-only; the student projection lives in attempts.go and
// never selects is_correct.
func (r *Repo) ListMcqQuestions(ctx context.Context, req *assessmentv1.ListMcqQuestionsRequest) (*assessmentv1.ListMcqQuestionsResponse, error) {
	page, pageSize, offset := clampPage(req.Page, req.PageSize)

	var clauses []string
	var args []any
	add := func(clause string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	if req.CompanyId != "" {
		// A company sees its own bank plus the shared platform bank.
		add("(company_id = $%d OR company_id IS NULL)", req.CompanyId)
	}
	// Deleted questions are soft-deleted (see DeleteMcqQuestion); the bank
	// screen should not keep showing them unless asked.
	if !req.IncludeRetired {
		clauses = append(clauses, "is_active = true")
	}
	switch req.CourseId {
	case "":
	case assessmentv1.McqGeneralCourse:
		clauses = append(clauses, "course_id = ''")
	default:
		add("course_id = $%d", req.CourseId)
	}
	if req.Topic != "" {
		add("topic = $%d", req.Topic)
	}
	if req.Difficulty != "" {
		add("difficulty = $%d", req.Difficulty)
	}
	if req.Search != "" {
		add("body ILIKE '%%' || $%d || '%%'", req.Search)
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	var total int32
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM mcq_questions "+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count mcq questions: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, COALESCE(company_id::text, ''), course_id, topic, difficulty, body, kind,
		       explanation, is_active, created_at
		FROM   mcq_questions %s
		ORDER  BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, len(args)+1, len(args)+2)

	rows, err := r.pool.Query(ctx, query, append(args, pageSize, offset)...)
	if err != nil {
		return nil, fmt.Errorf("list mcq questions: %w", err)
	}
	defer rows.Close()

	out := &assessmentv1.ListMcqQuestionsResponse{
		Questions: []*assessmentv1.McqQuestion{},
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
	}
	ids := []string{}
	byID := map[string]*assessmentv1.McqQuestion{}

	for rows.Next() {
		q := &assessmentv1.McqQuestion{Options: []*assessmentv1.McqOption{}}
		var created any
		if err := rows.Scan(&q.Id, &q.CompanyId, &q.CourseId, &q.Topic, &q.Difficulty, &q.Body,
			&q.Kind, &q.Explanation, &q.IsActive, &created); err != nil {
			return nil, fmt.Errorf("scan mcq question: %w", err)
		}
		out.Questions = append(out.Questions, q)
		ids = append(ids, q.Id)
		byID[q.Id] = q
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return out, nil
	}

	optRows, err := r.pool.Query(ctx, `
		SELECT question_id, id, body, is_correct, order_index
		FROM   mcq_options
		WHERE  question_id = ANY($1::uuid[])
		ORDER  BY question_id, order_index
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("load mcq options: %w", err)
	}
	defer optRows.Close()

	for optRows.Next() {
		var qid string
		o := &assessmentv1.McqOption{}
		if err := optRows.Scan(&qid, &o.Id, &o.Body, &o.IsCorrect, &o.OrderIndex); err != nil {
			return nil, fmt.Errorf("scan mcq option: %w", err)
		}
		if q, ok := byID[qid]; ok {
			q.Options = append(q.Options, o)
		}
	}
	return out, optRows.Err()
}

// DeleteMcqQuestion retires a question. It is a soft delete: a hard delete
// would either break the foreign key from a published test or silently rewrite
// history for attempts that already used it.
func (r *Repo) DeleteMcqQuestion(ctx context.Context, id string) error {
	ct, err := r.pool.Exec(ctx, `UPDATE mcq_questions SET is_active = false WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("retire mcq question: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// BulkImportMcq inserts a batch from a spreadsheet upload. Bad rows are
// reported individually instead of failing the whole import, because a single
// malformed row in a 500-question sheet should not cost the whole upload.
func (r *Repo) BulkImportMcq(ctx context.Context, req *assessmentv1.BulkImportMcqRequest) (*assessmentv1.BulkImportMcqResponse, error) {
	out := &assessmentv1.BulkImportMcqResponse{}
	for i, q := range req.Questions {
		if q == nil {
			continue
		}
		q.Id = "" // an import always creates
		if q.CompanyId == "" {
			q.CompanyId = req.CompanyId
		}
		if _, err := r.UpsertMcqQuestion(ctx, &assessmentv1.UpsertMcqQuestionRequest{
			ActorId:  req.ActorId,
			Question: q,
		}); err != nil {
			out.Failed++
			if len(out.Errors) < 25 { // cap the payload; the count stays exact
				out.Errors = append(out.Errors, fmt.Sprintf("row %d: %v", i+1, err))
			}
			continue
		}
		out.Imported++
	}
	if out.Failed > int32(len(out.Errors)) {
		out.Errors = append(out.Errors, fmt.Sprintf("... and %d more failed rows", out.Failed-int32(len(out.Errors))))
	}
	return out, nil
}

func defaultStr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
