// Package repository provides Postgres-backed data access for the submission service.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	submissionv1 "github.com/knovate211/proto/submission/v1"
)

// SubmissionRepository wraps a pgxpool for submission CRUD.
type SubmissionRepository struct {
	pool *pgxpool.Pool
}

// New constructs a SubmissionRepository.
func New(pool *pgxpool.Pool) *SubmissionRepository {
	return &SubmissionRepository{pool: pool}
}

// CreateSubmission inserts a new submission with Pending status and returns its ID.
func (r *SubmissionRepository) CreateSubmission(ctx context.Context, req *submissionv1.SubmitRequest) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO submissions (user_id, problem_id, language, code, status, submitted_at)
		VALUES ($1, $2, $3, $4, 'Pending', $5)
		RETURNING id::text
	`, req.UserId, req.ProblemId, req.Language, req.Code, time.Now().UTC()).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create submission: %w", err)
	}
	return id, nil
}

// UpdateSubmissionResult updates a submission after execution completes.
func (r *SubmissionRepository) UpdateSubmissionResult(ctx context.Context, id, status string, runtimeMs, memoryKb int64, compileError string, testResults []*submissionv1.TestResult) error {
	trJSON, err := json.Marshal(testResults)
	if err != nil {
		return fmt.Errorf("marshal test results: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE submissions
		SET    status        = $1,
		       runtime_ms    = $2,
		       memory_kb     = $3,
		       compile_error = $4,
		       test_results  = $5,
		       completed_at  = $6
		WHERE  id::text = $7
	`, status, runtimeMs, memoryKb, compileError, trJSON, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update submission result: %w", err)
	}
	return nil
}

// GetSubmission retrieves a single submission by ID.
func (r *SubmissionRepository) GetSubmission(ctx context.Context, id string) (*submissionv1.Submission, error) {
	s := &submissionv1.Submission{}
	var trJSON []byte
	var completedAt *time.Time
	var submittedAt time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id::text, user_id, problem_id::text, language, code,
		       status, runtime_ms, memory_kb, COALESCE(compile_error,''),
		       test_results, submitted_at, completed_at
		FROM   submissions
		WHERE  id::text = $1
	`, id).Scan(
		&s.Id, &s.UserId, &s.ProblemId, &s.Language, &s.Code,
		&s.Status, &s.RuntimeMs, &s.MemoryKb, &s.CompileError,
		&trJSON, &submittedAt, &completedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("submission not found: %s", id)
		}
		return nil, fmt.Errorf("get submission: %w", err)
	}

	s.SubmittedAt = submittedAt.UTC().Format(time.RFC3339)
	if completedAt != nil {
		s.CompletedAt = completedAt.UTC().Format(time.RFC3339)
	}

	if len(trJSON) > 0 {
		json.Unmarshal(trJSON, &s.TestResults) //nolint:errcheck
	}

	return s, nil
}

// ListSubmissions returns paginated submissions filtered by user and optional problem.
func (r *SubmissionRepository) ListSubmissions(ctx context.Context, req *submissionv1.ListSubmissionsRequest) (*submissionv1.ListSubmissionsResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	args := []interface{}{req.UserId}
	where := "WHERE user_id = $1"

	if req.ProblemId != "" {
		args = append(args, req.ProblemId)
		where += fmt.Sprintf(" AND problem_id::text = $%d", len(args))
	}

	args = append(args, pageSize, offset)
	limitOffset := fmt.Sprintf("LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	query := fmt.Sprintf(`
		SELECT id::text, user_id, problem_id::text, language, status,
		       runtime_ms, memory_kb, submitted_at, COALESCE(completed_at::text,'')
		FROM   submissions
		%s
		ORDER  BY submitted_at DESC
		%s
	`, where, limitOffset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}
	defer rows.Close()

	var submissions []*submissionv1.Submission
	for rows.Next() {
		s := &submissionv1.Submission{}
		var submittedAt time.Time
		if err := rows.Scan(
			&s.Id, &s.UserId, &s.ProblemId, &s.Language, &s.Status,
			&s.RuntimeMs, &s.MemoryKb, &submittedAt, &s.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan submission: %w", err)
		}
		s.SubmittedAt = submittedAt.UTC().Format(time.RFC3339)
		submissions = append(submissions, s)
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM submissions %s", where)
	var total int32
	r.pool.QueryRow(ctx, countQuery, args[:len(args)-2]...).Scan(&total) //nolint:errcheck

	return &submissionv1.ListSubmissionsResponse{
		Submissions: submissions,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}
