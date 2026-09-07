package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/knovate211/assessment-service/internal/grading"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// FinalizeAttempt closes an attempt and grades everything that can be graded
// synchronously. Coding questions whose judge verdict has not arrived yet are
// left pending and the attempt sits in `evaluating` until the graded consumer
// completes them.
//
// It is idempotent — a double submit, a timeout racing a manual submit, or the
// sweeper firing during a submit all converge on the same result.
func (r *Repo) FinalizeAttempt(ctx context.Context, attemptID, reason string) (*assessmentv1.AttemptSummary, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin finalize: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	a, err := r.loadAttempt(ctx, tx, attemptID, true)
	if err != nil {
		return nil, err
	}
	if a.Status != "in_progress" {
		// Already closed by another path; report what is on record.
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit finalize: %w", err)
		}
		return r.AttemptSummary(ctx, attemptID)
	}

	if err := r.gradeObjective(ctx, tx, attemptID, a.NegativeMarking); err != nil {
		return nil, err
	}

	status := "submitted"
	if reason == "proctor" {
		status = "disqualified"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE attempts SET status = $2, submitted_at = now() WHERE id = $1
	`, attemptID, status); err != nil {
		return nil, fmt.Errorf("close attempt: %w", err)
	}

	if err := r.recompute(ctx, tx, attemptID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit finalize: %w", err)
	}
	return r.AttemptSummary(ctx, attemptID)
}

// gradeObjective marks every MCQ in the attempt against the frozen answer key.
// Descriptive answers are deliberately untouched: they stay in manual_review
// for a human.
func (r *Repo) gradeObjective(ctx context.Context, q querier, attemptID string, negativeMarking float64) error {
	rows, err := q.Query(ctx, `
		SELECT aq.id::text, aq.marks, COALESCE(m.kind, 'single'), s.partial_credit,
		       aq.selected_options::text[], COALESCE(aq.text_answer, ''),
		       COALESCE(array_agg(o.id::text)   FILTER (WHERE o.is_correct), '{}') AS correct_ids,
		       COALESCE(array_agg(o.body)       FILTER (WHERE o.is_correct), '{}') AS correct_bodies
		FROM   attempt_questions aq
		JOIN   assessment_sections s ON s.id = aq.section_id
		LEFT   JOIN mcq_questions m ON m.id = aq.mcq_question_id
		LEFT   JOIN mcq_options   o ON o.question_id = aq.mcq_question_id
		WHERE  aq.attempt_id = $1 AND aq.kind = 'mcq'
		GROUP  BY aq.id, aq.marks, m.kind, s.partial_credit, aq.selected_options, aq.text_answer
	`, attemptID)
	if err != nil {
		return fmt.Errorf("load mcq answers: %w", err)
	}

	type scored struct {
		id    string
		marks float64
	}
	var results []scored

	for rows.Next() {
		var in grading.McqInput
		var id string
		if err := rows.Scan(&id, &in.Marks, &in.Kind, &in.PartialCredit,
			&in.SelectedIDs, &in.TextAnswer, &in.CorrectIDs, &in.AcceptedAnswers); err != nil {
			rows.Close()
			return fmt.Errorf("scan mcq answer: %w", err)
		}
		in.NegativeMarking = negativeMarking
		results = append(results, scored{id: id, marks: grading.ScoreMcq(in)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, s := range results {
		if _, err := q.Exec(ctx, `
			UPDATE attempt_questions SET awarded_marks = $2, grading_status = 'graded' WHERE id = $1
		`, s.id, s.marks); err != nil {
			return fmt.Errorf("save mcq mark: %w", err)
		}
	}
	return nil
}

// recompute rolls per-question marks up into the attempt total and the
// per-section breakdown, and settles the attempt status.
//
// The total is clamped at zero: negative marking can sink an individual
// question, but a candidate never finishes with less than nothing.
func (r *Repo) recompute(ctx context.Context, q querier, attemptID string) error {
	var total float64
	var pending, manual int
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(COALESCE(awarded_marks, 0)), 0),
		       COUNT(*) FILTER (WHERE grading_status = 'pending'),
		       COUNT(*) FILTER (WHERE grading_status = 'manual_review')
		FROM   attempt_questions WHERE attempt_id = $1
	`, attemptID).Scan(&total, &pending, &manual); err != nil {
		return fmt.Errorf("aggregate attempt marks: %w", err)
	}

	rows, err := q.Query(ctx, `
		SELECT section_id::text, COALESCE(SUM(COALESCE(awarded_marks, 0)), 0)
		FROM   attempt_questions WHERE attempt_id = $1 GROUP BY section_id
	`, attemptID)
	if err != nil {
		return fmt.Errorf("aggregate section marks: %w", err)
	}
	sections := map[string]float64{}
	for rows.Next() {
		var id string
		var marks float64
		if err := rows.Scan(&id, &marks); err != nil {
			rows.Close()
			return fmt.Errorf("scan section marks: %w", err)
		}
		sections[id] = marks
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	sectionJSON, err := json.Marshal(sections)
	if err != nil {
		return fmt.Errorf("marshal section scores: %w", err)
	}

	// A pending judge verdict keeps the attempt in `evaluating`. Pending manual
	// review does not: the recruiter needs to see the objective score
	// immediately, and grading a descriptive answer recomputes the total.
	_, err = q.Exec(ctx, `
		UPDATE attempts SET
			score          = GREATEST($2, 0),
			section_scores = $3,
			status         = CASE
				WHEN status IN ('submitted', 'evaluating', 'evaluated')
					THEN (CASE WHEN $4 THEN 'evaluating' ELSE 'evaluated' END)
				ELSE status
			END,
			evaluated_at   = CASE
				WHEN status IN ('submitted', 'evaluating', 'evaluated') AND NOT $4 THEN now()
				ELSE evaluated_at
			END
		WHERE id = $1
	`, attemptID, total, sectionJSON, pending > 0)
	if err != nil {
		return fmt.Errorf("update attempt score: %w", err)
	}
	return nil
}

// ApplyGradedSubmission routes a submission.graded event back to the attempt
// question that produced it.
//
// A question keeps the BEST of its submissions, not the last: a candidate who
// scores 8/10 and then experiments with a rewrite that scores 2/10 before the
// clock runs out should not be punished for exploring.
//
// Submissions that belong to ordinary practice (not an attempt) match no row
// here and are ignored.
func (r *Repo) ApplyGradedSubmission(ctx context.Context, submissionID, status string, passed, total int32) error {
	var questionID string
	err := r.pool.QueryRow(ctx, `
		UPDATE attempt_submissions
		SET    status = $2, passed_count = $3, total_count = $4
		WHERE  submission_id = $1
		RETURNING attempt_question_id::text
	`, submissionID, status, passed, total).Scan(&questionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // not an assessment submission
	}
	if err != nil {
		return fmt.Errorf("update attempt submission: %w", err)
	}

	var attemptID string
	var marks float64
	if err := r.pool.QueryRow(ctx, `
		SELECT attempt_id::text, marks FROM attempt_questions WHERE id = $1
	`, questionID).Scan(&attemptID, &marks); err != nil {
		return fmt.Errorf("load attempt question for grading: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT submission_id::text, language, code, COALESCE(passed_count, 0), COALESCE(total_count, 0)
		FROM   attempt_submissions
		WHERE  attempt_question_id = $1 AND passed_count IS NOT NULL
	`, questionID)
	if err != nil {
		return fmt.Errorf("load attempt submissions: %w", err)
	}

	best := struct {
		score            float64
		subID, lang, src string
		found            bool
	}{}
	for rows.Next() {
		var subID, lang, src string
		var p, t int32
		if err := rows.Scan(&subID, &lang, &src, &p, &t); err != nil {
			rows.Close()
			return fmt.Errorf("scan attempt submission: %w", err)
		}
		if score := grading.ScoreCoding(int(p), int(t), marks); !best.found || score > best.score {
			best = struct {
				score            float64
				subID, lang, src string
				found            bool
			}{score, subID, lang, src, true}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if !best.found {
		return nil
	}

	if _, err := r.pool.Exec(ctx, `
		UPDATE attempt_questions
		SET    awarded_marks = $2, grading_status = 'graded',
		       submission_id = $3, language = $4, code = $5
		WHERE  id = $1
	`, questionID, best.score, best.subID, best.lang, best.src); err != nil {
		return fmt.Errorf("save coding mark: %w", err)
	}

	return r.recompute(ctx, r.pool, attemptID)
}

// GradeDescriptive records a recruiter's manual mark and rescores the attempt.
func (r *Repo) GradeDescriptive(ctx context.Context, req *assessmentv1.GradeDescriptiveRequest) (*assessmentv1.GradeDescriptiveResponse, error) {
	var maxMarks float64
	err := r.pool.QueryRow(ctx, `
		SELECT marks FROM attempt_questions
		WHERE id = $1 AND attempt_id = $2 AND kind = 'descriptive'
	`, req.QuestionId, req.AttemptId).Scan(&maxMarks)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup descriptive question: %w", err)
	}
	if req.Marks < 0 || req.Marks > maxMarks {
		return nil, fmt.Errorf("marks must be between 0 and %g", maxMarks)
	}

	if _, err := r.pool.Exec(ctx, `
		UPDATE attempt_questions SET awarded_marks = $2, grading_status = 'graded' WHERE id = $1
	`, req.QuestionId, req.Marks); err != nil {
		return nil, fmt.Errorf("save descriptive mark: %w", err)
	}
	if err := r.recompute(ctx, r.pool, req.AttemptId); err != nil {
		return nil, err
	}

	s, err := r.AttemptSummary(ctx, req.AttemptId)
	if err != nil {
		return nil, err
	}
	return &assessmentv1.GradeDescriptiveResponse{Score: s.Score, MaxScore: s.MaxScore}, nil
}

// ExpireDueAttempts finalizes attempts whose deadline has passed. It backstops
// the on-request expiry check for candidates who simply close the tab — without
// it their results would sit unscored forever.
func (r *Repo) ExpireDueAttempts(ctx context.Context) (int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text FROM attempts
		WHERE  status = 'in_progress' AND expires_at < now()
		LIMIT  200
	`)
	if err != nil {
		return 0, fmt.Errorf("find expired attempts: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan expired attempt: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	closed := 0
	for _, id := range ids {
		if _, err := r.FinalizeAttempt(ctx, id, "timeout"); err != nil {
			// One bad attempt must not stall the sweep for the rest.
			continue
		}
		closed++
	}
	return closed, nil
}

// PendingGradingCount reports how many coding questions in an attempt are still
// waiting on the judge, so the result page can say "still evaluating".
func (r *Repo) PendingGradingCount(ctx context.Context, attemptID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM attempt_questions
		WHERE attempt_id = $1 AND grading_status = 'pending'
	`, attemptID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count pending grading: %w", err)
	}
	return n, nil
}

// ─── Summaries & results ──────────────────────────────────────────────────────

// AttemptSummary is the one-row view used by result pages and recruiter tables.
func (r *Repo) AttemptSummary(ctx context.Context, attemptID string) (*assessmentv1.AttemptSummary, error) {
	rows, err := r.attemptSummaries(ctx, `WHERE at.id = $1`, []any{attemptID}, "", 0, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return rows[0], nil
}

// attemptSummaries is the shared summary query behind AttemptSummary,
// ListMyAttempts, ListAttempts and shortlisting.
func (r *Repo) attemptSummaries(ctx context.Context, where string, args []any, orderBy string, limit, offset int32) ([]*assessmentv1.AttemptSummary, error) {
	if orderBy == "" {
		orderBy = "at.started_at DESC"
	}
	pagination := ""
	if limit > 0 {
		pagination = fmt.Sprintf("LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	query := fmt.Sprintf(`
		SELECT at.id::text, at.assessment_id::text, a.title, at.user_id::text,
		       COALESCE(u.name, ''), COALESCE(u.email, ''), at.attempt_no, at.status,
		       at.started_at, at.submitted_at, at.evaluated_at,
		       at.score, at.max_score, at.integrity_score, a.passing_marks,
		       at.section_scores, a.purpose,
		       COALESCE((
		         SELECT jsonb_object_agg(x.section_id, x.total)
		         FROM (
		           SELECT aq.section_id::text AS section_id, SUM(aq.marks) AS total
		           FROM attempt_questions aq WHERE aq.attempt_id = at.id
		           GROUP BY aq.section_id
		         ) x
		       ), '{}'::jsonb),
		       COALESCE((
		         SELECT se.decision FROM shortlist_entries se
		         WHERE se.attempt_id = at.id ORDER BY se.rank LIMIT 1
		       ), '')
		FROM   attempts at
		JOIN   assessments a ON a.id = at.assessment_id
		LEFT   JOIN users u ON u.id = at.user_id
		%s
		ORDER  BY %s
		%s
	`, where, orderBy, pagination)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list attempt summaries: %w", err)
	}
	defer rows.Close()

	out := []*assessmentv1.AttemptSummary{}
	for rows.Next() {
		s := &assessmentv1.AttemptSummary{}
		var startedAt time.Time
		var submittedAt, evaluatedAt *time.Time
		var passingMarks float64
		var sectionScores, sectionMax []byte

		if err := rows.Scan(&s.Id, &s.AssessmentId, &s.AssessmentName, &s.UserId,
			&s.UserName, &s.UserEmail, &s.AttemptNo, &s.Status,
			&startedAt, &submittedAt, &evaluatedAt,
			&s.Score, &s.MaxScore, &s.IntegrityScore, &passingMarks,
			&sectionScores, &s.Purpose, &sectionMax, &s.Decision); err != nil {
			return nil, fmt.Errorf("scan attempt summary: %w", err)
		}

		s.StartedAt = fmtTime(&startedAt)
		s.SubmittedAt, s.EvaluatedAt = fmtTime(submittedAt), fmtTime(evaluatedAt)
		s.SectionScores, s.SectionMax = map[string]float64{}, map[string]float64{}
		fromJSON(sectionScores, &s.SectionScores)
		fromJSON(sectionMax, &s.SectionMax)
		if s.MaxScore > 0 {
			s.Percent = round2(s.Score / s.MaxScore * 100)
		}
		s.Passed = passingMarks > 0 && s.Score >= passingMarks
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListMyAttempts returns a student's own attempt history.
func (r *Repo) ListMyAttempts(ctx context.Context, userID string) ([]*assessmentv1.AttemptSummary, error) {
	out, err := r.attemptSummaries(ctx, `WHERE at.user_id = $1`, []any{userID}, "", 0, 0)
	if err != nil {
		return nil, err
	}
	// This is the one summary list a candidate reads about themselves, so it is
	// also a way to learn a withheld score. Strip the marks here rather than in
	// the caller: every future caller of "my attempts" inherits the redaction.
	for _, s := range out {
		redactWithheld(s)
	}
	return out, nil
}

// redactWithheld strips the marks from a summary the candidate may not see.
//
// It clears the numbers rather than dropping the row: the candidate should
// still be told the paper is submitted and when — silence after an hour of work
// reads as a system failure.
func redactWithheld(s *assessmentv1.AttemptSummary) {
	if s == nil || !resultsWithheld(s.Purpose) {
		return
	}
	s.Score, s.MaxScore, s.Percent, s.Passed = 0, 0, 0, false
	s.SectionScores, s.SectionMax, s.Decision = nil, nil, ""
}

// GetAttemptResult returns a student's own result. The question-by-question
// breakdown — and with it the answer key — is attached only when the
// assessment explicitly allows revealing results.
func (r *Repo) GetAttemptResult(ctx context.Context, attemptID, userID string) (*assessmentv1.AttemptResult, error) {
	a, err := r.loadAttempt(ctx, r.pool, attemptID, false)
	if err != nil {
		return nil, err
	}
	if a.UserID != userID {
		return nil, ErrForbidden
	}
	if a.Status == "in_progress" {
		return nil, fmt.Errorf("this attempt is still in progress")
	}

	summary, err := r.AttemptSummary(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	// Withholding happens before anything else is attached. Returning early
	// with a redacted summary means there is no path from here to a mark, a
	// percentage or an answer key, whatever reveal_results happens to say.
	if resultsWithheld(a.Purpose) {
		redactWithheld(summary)
		return &assessmentv1.AttemptResult{Summary: summary, Withheld: true}, nil
	}

	out := &assessmentv1.AttemptResult{Summary: summary, Revealed: a.RevealResults}
	if !a.RevealResults {
		return out, nil
	}
	if out.Questions, err = r.attemptQuestions(ctx, attemptID, true); err != nil {
		return nil, err
	}
	return out, nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
