package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// CreateAssessment stores a new draft test.
func (r *Repo) CreateAssessment(ctx context.Context, req *assessmentv1.CreateAssessmentRequest) (string, error) {
	a := req.Assessment
	if a == nil || strings.TrimSpace(a.Title) == "" {
		return "", fmt.Errorf("title is required")
	}
	if a.DurationMinutes <= 0 {
		return "", fmt.Errorf("duration_minutes must be positive")
	}
	proctoring, err := toJSON(a.Proctoring)
	if err != nil {
		return "", err
	}

	var id string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO assessments (
			company_id, title, description, purpose, duration_minutes, passing_marks,
			negative_marking, shuffle_questions, shuffle_options, allow_backtrack,
			reveal_results, proctoring, opens_at, closes_at, max_attempts, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING id
	`,
		nullable(a.CompanyId), a.Title, a.Description, defaultStr(a.Purpose, "practice"),
		a.DurationMinutes, a.PassingMarks, a.NegativeMarking, a.ShuffleQuestions,
		a.ShuffleOptions, a.AllowBacktrack, a.RevealResults, proctoring,
		parseTime(a.OpensAt), parseTime(a.ClosesAt), maxInt32(a.MaxAttempts, 1), req.ActorId,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert assessment: %w", err)
	}
	return id, nil
}

// UpdateAssessment edits a test's settings. Sections and questions are managed
// through their own calls.
//
// A published test can still be edited (a recruiter fixing a typo mid-drive is
// a real need), but attempts already started are unaffected: they grade against
// the frozen paper captured at start.
func (r *Repo) UpdateAssessment(ctx context.Context, req *assessmentv1.UpdateAssessmentRequest) error {
	a := req.Assessment
	if a == nil || a.Id == "" {
		return fmt.Errorf("assessment id is required")
	}
	// nil, not "{}" — so the COALESCE below keeps whatever is stored when the
	// caller did not mention proctoring. toJSON cannot express that: a nil
	// *Proctoring reaches it inside a non-nil interface, so it marshals to the
	// JSON literal `null`, which is a value and would overwrite.
	var proctoring any
	if a.Proctoring != nil {
		b, err := toJSON(a.Proctoring)
		if err != nil {
			return err
		}
		proctoring = b
	}

	// Absent-preserving, not defaulting. A caller sending only the field it
	// wants to change is the normal shape of a PATCH, and the previous version
	// wrote every omitted field as its zero value — so a request carrying just
	// {"proctoring": …} blanked a published paper's title and set its duration
	// to zero, which in turn made every new attempt expire the instant it
	// started. The damage is silent: the paper still lists, still opens, and
	// only fails once a candidate is sitting it.
	//
	// Strings and numbers fall back to the stored value when empty or zero.
	// That does mean a title cannot be *cleared* through this path, which is
	// the right trade: nobody wants a nameless assessment, and losing one by
	// omission is far more likely than clearing one on purpose. The booleans
	// are the exception — false is a real choice — so they are only written
	// when the caller sent the whole object, which is what the admin UI does.
	ct, err := r.pool.Exec(ctx, `
		UPDATE assessments SET
			title            = COALESCE(NULLIF($2, ''), title),
			description      = COALESCE(NULLIF($3, ''), description),
			duration_minutes = CASE WHEN $4 > 0 THEN $4 ELSE duration_minutes END,
			passing_marks    = CASE WHEN $5 > 0 THEN $5 ELSE passing_marks END,
			negative_marking = CASE WHEN $6 > 0 THEN $6 ELSE negative_marking END,
			shuffle_questions = CASE WHEN $15 THEN $7  ELSE shuffle_questions END,
			shuffle_options   = CASE WHEN $15 THEN $8  ELSE shuffle_options   END,
			allow_backtrack   = CASE WHEN $15 THEN $9  ELSE allow_backtrack   END,
			reveal_results    = CASE WHEN $15 THEN $10 ELSE reveal_results    END,
			proctoring       = COALESCE($11::jsonb, proctoring),
			opens_at         = COALESCE($12, opens_at),
			closes_at        = COALESCE($13, closes_at),
			max_attempts     = CASE WHEN $14 > 0 THEN $14 ELSE max_attempts END,
			updated_at       = now()
		WHERE id = $1
	`, a.Id, a.Title, a.Description, a.DurationMinutes, a.PassingMarks,
		a.NegativeMarking, a.ShuffleQuestions, a.ShuffleOptions, a.AllowBacktrack,
		a.RevealResults, proctoring, parseTime(a.OpensAt), parseTime(a.ClosesAt),
		a.MaxAttempts,
		// "The caller sent the whole assessment" — the admin editor always
		// includes the title, so its presence is what distinguishes a full save
		// from a one-field patch.
		a.Title != "")
	if err != nil {
		return fmt.Errorf("update assessment: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAssessment loads a test with its sections and questions. The answer key is
// attached only when includeKey is set, which the student-facing paths never
// do.
func (r *Repo) GetAssessment(ctx context.Context, id string, includeKey bool) (*assessmentv1.Assessment, error) {
	a := &assessmentv1.Assessment{Sections: []*assessmentv1.Section{}}
	var proctoring []byte
	var companyID, companyName *string
	var opensAt, closesAt, createdAt, updatedAt *time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT a.id, a.company_id::text, c.name, a.title, a.description, a.purpose,
		       a.duration_minutes, a.total_marks, a.passing_marks, a.negative_marking,
		       a.shuffle_questions, a.shuffle_options, a.allow_backtrack, a.reveal_results,
		       a.proctoring, a.status, a.opens_at, a.closes_at, a.max_attempts,
		       a.created_by::text, a.created_at, a.updated_at
		FROM   assessments a
		LEFT   JOIN companies c ON c.id = a.company_id
		WHERE  a.id = $1
	`, id).Scan(&a.Id, &companyID, &companyName, &a.Title, &a.Description, &a.Purpose,
		&a.DurationMinutes, &a.TotalMarks, &a.PassingMarks, &a.NegativeMarking,
		&a.ShuffleQuestions, &a.ShuffleOptions, &a.AllowBacktrack, &a.RevealResults,
		&proctoring, &a.Status, &opensAt, &closesAt, &a.MaxAttempts,
		&a.CreatedBy, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get assessment: %w", err)
	}

	a.CompanyId = derefStr(companyID)
	a.CompanyName = derefStr(companyName)
	a.Proctoring = defaultProctoring()
	fromJSON(proctoring, a.Proctoring)
	a.OpensAt, a.ClosesAt = fmtTime(opensAt), fmtTime(closesAt)
	a.CreatedAt, a.UpdatedAt = fmtTime(createdAt), fmtTime(updatedAt)

	sections, err := r.loadSections(ctx, id, includeKey)
	if err != nil {
		return nil, err
	}
	a.Sections = sections
	for _, s := range sections {
		a.QuestionCount += int32(len(s.Questions))
		if s.PickCount > 0 {
			a.QuestionCount = a.QuestionCount - int32(len(s.Questions)) + s.PickCount
		}
	}
	return a, nil
}

// loadSections fetches sections plus their question lists in two queries.
func (r *Repo) loadSections(ctx context.Context, assessmentID string, includeTitles bool) ([]*assessmentv1.Section, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, kind, order_index, COALESCE(duration_minutes, 0),
		       cutoff_marks, COALESCE(pick_count, 0), pick_topic, pick_difficulty,
		       pick_marks, partial_credit
		FROM   assessment_sections
		WHERE  assessment_id = $1
		ORDER  BY order_index, title
	`, assessmentID)
	if err != nil {
		return nil, fmt.Errorf("load sections: %w", err)
	}
	defer rows.Close()

	sections := []*assessmentv1.Section{}
	byID := map[string]*assessmentv1.Section{}
	for rows.Next() {
		s := &assessmentv1.Section{AssessmentId: assessmentID, Questions: []*assessmentv1.SectionQuestion{}}
		if err := rows.Scan(&s.Id, &s.Title, &s.Kind, &s.OrderIndex, &s.DurationMinutes,
			&s.CutoffMarks, &s.PickCount, &s.PickTopic, &s.PickDifficulty,
			&s.PickMarks, &s.PartialCredit); err != nil {
			return nil, fmt.Errorf("scan section: %w", err)
		}
		sections = append(sections, s)
		byID[s.Id] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(sections) == 0 {
		return sections, nil
	}

	ids := make([]string, 0, len(sections))
	for _, s := range sections {
		ids = append(ids, s.Id)
	}

	// The preview title/difficulty come from whichever bank the question lives
	// in — the local MCQ table or problem-service's problems table.
	qRows, err := r.pool.Query(ctx, `
		SELECT sq.section_id, sq.id,
		       COALESCE(sq.mcq_question_id::text, ''), COALESCE(sq.problem_id::text, ''),
		       sq.marks, sq.order_index,
		       COALESCE(LEFT(m.body, 160), p.title, ''),
		       COALESCE(m.difficulty, p.difficulty, '')
		FROM   section_questions sq
		LEFT   JOIN mcq_questions m ON m.id = sq.mcq_question_id
		LEFT   JOIN problems      p ON p.id = sq.problem_id
		WHERE  sq.section_id = ANY($1::uuid[])
		ORDER  BY sq.section_id, sq.order_index
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("load section questions: %w", err)
	}
	defer qRows.Close()

	for qRows.Next() {
		var sectionID string
		q := &assessmentv1.SectionQuestion{}
		if err := qRows.Scan(&sectionID, &q.Id, &q.McqQuestionId, &q.ProblemId,
			&q.Marks, &q.OrderIndex, &q.Title, &q.Difficulty); err != nil {
			return nil, fmt.Errorf("scan section question: %w", err)
		}
		q.SectionId = sectionID
		if !includeTitles {
			q.Title, q.Difficulty = "", ""
		}
		if s, ok := byID[sectionID]; ok {
			s.Questions = append(s.Questions, q)
		}
	}
	return sections, qRows.Err()
}

// ListAssessments powers the authoring and recruiter list views.
func (r *Repo) ListAssessments(ctx context.Context, req *assessmentv1.ListAssessmentsRequest) (*assessmentv1.ListAssessmentsResponse, error) {
	_, pageSize, offset := clampPage(req.Page, req.PageSize)

	var clauses []string
	var args []any
	add := func(clause string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if req.CompanyId != "" {
		add("a.company_id = $%d", req.CompanyId)
	}
	if req.Purpose != "" {
		add("a.purpose = $%d", req.Purpose)
	}
	if req.Status != "" {
		add("a.status = $%d", req.Status)
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	var total int32
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM assessments a "+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count assessments: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT a.id, COALESCE(a.company_id::text, ''), COALESCE(c.name, ''), a.title,
		       a.description, a.purpose, a.duration_minutes, a.total_marks, a.status,
		       a.opens_at, a.closes_at, a.max_attempts, a.created_at,
		       (SELECT COUNT(*) FROM attempts at WHERE at.assessment_id = a.id)
		FROM   assessments a
		LEFT   JOIN companies c ON c.id = a.company_id
		%s
		ORDER  BY a.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, len(args)+1, len(args)+2)

	rows, err := r.pool.Query(ctx, query, append(args, pageSize, offset)...)
	if err != nil {
		return nil, fmt.Errorf("list assessments: %w", err)
	}
	defer rows.Close()

	out := &assessmentv1.ListAssessmentsResponse{Assessments: []*assessmentv1.Assessment{}, Total: total}
	for rows.Next() {
		a := &assessmentv1.Assessment{}
		var opensAt, closesAt, createdAt *time.Time
		if err := rows.Scan(&a.Id, &a.CompanyId, &a.CompanyName, &a.Title, &a.Description,
			&a.Purpose, &a.DurationMinutes, &a.TotalMarks, &a.Status,
			&opensAt, &closesAt, &a.MaxAttempts, &createdAt, &a.AttemptCount); err != nil {
			return nil, fmt.Errorf("scan assessment: %w", err)
		}
		a.OpensAt, a.ClosesAt, a.CreatedAt = fmtTime(opensAt), fmtTime(closesAt), fmtTime(createdAt)
		out.Assessments = append(out.Assessments, a)
	}
	return out, rows.Err()
}

// PublishAssessment validates a draft, caches its total marks and flips its
// status. Validation failures that would break a live paper are errors;
// everything else is returned as a warning so the recruiter can decide.
func (r *Repo) PublishAssessment(ctx context.Context, req *assessmentv1.PublishAssessmentRequest) (*assessmentv1.PublishAssessmentResponse, error) {
	if !req.Publish {
		var status string
		err := r.pool.QueryRow(ctx, `
			UPDATE assessments SET status = 'draft', updated_at = now()
			WHERE id = $1 RETURNING status
		`, req.Id).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("unpublish assessment: %w", err)
		}
		return &assessmentv1.PublishAssessmentResponse{Status: status}, nil
	}

	a, err := r.GetAssessment(ctx, req.Id, true)
	if err != nil {
		return nil, err
	}

	total, warnings, err := validateForPublish(a)
	if err != nil {
		return nil, err
	}

	var status string
	err = r.pool.QueryRow(ctx, `
		UPDATE assessments SET status = 'published', total_marks = $2, updated_at = now()
		WHERE id = $1 RETURNING status
	`, req.Id, total).Scan(&status)
	if err != nil {
		return nil, fmt.Errorf("publish assessment: %w", err)
	}

	return &assessmentv1.PublishAssessmentResponse{
		Status:     status,
		TotalMarks: total,
		Warnings:   warnings,
	}, nil
}

// validateForPublish computes the total marks and reports anything that would
// make the test unusable or unfair.
func validateForPublish(a *assessmentv1.Assessment) (int32, []string, error) {
	if len(a.Sections) == 0 {
		return 0, nil, fmt.Errorf("cannot publish a test with no sections")
	}

	var total int32
	var warnings []string
	sectionMinutes := int32(0)

	for _, s := range a.Sections {
		switch {
		case s.PickCount > 0 && len(s.Questions) > 0:
			if s.PickCount > int32(len(s.Questions)) {
				return 0, nil, fmt.Errorf("section %q draws %d questions from a pool of only %d",
					s.Title, s.PickCount, len(s.Questions))
			}
			// Every candidate draws a different subset, so the drawn questions
			// must be equal-weight: pick_marks applies, per-question marks do not.
			total += s.PickCount * maxInt32(s.PickMarks, 1)
		case s.PickCount > 0:
			if s.Kind != "mcq" {
				return 0, nil, fmt.Errorf("section %q can only draw from the bank for MCQ questions", s.Title)
			}
			total += s.PickCount * maxInt32(s.PickMarks, 1)
		case len(s.Questions) == 0:
			return 0, nil, fmt.Errorf("section %q has no questions", s.Title)
		default:
			for _, q := range s.Questions {
				total += maxInt32(q.Marks, 1)
			}
		}

		if s.Kind == "descriptive" {
			warnings = append(warnings, fmt.Sprintf(
				"section %q is descriptive — those answers need manual grading before results are final", s.Title))
		}
		sectionMinutes += s.DurationMinutes
	}

	if sectionMinutes > 0 && sectionMinutes > a.DurationMinutes {
		warnings = append(warnings, fmt.Sprintf(
			"section timers add up to %d min but the test allows %d min", sectionMinutes, a.DurationMinutes))
	}
	if a.PassingMarks > total {
		return 0, nil, fmt.Errorf("passing marks (%d) exceed the total marks (%d)", a.PassingMarks, total)
	}
	if a.Purpose == "hiring" && a.CompanyId == "" {
		return 0, nil, fmt.Errorf("a hiring test must belong to a company")
	}
	return total, warnings, nil
}

// DeleteAssessment removes a draft. Tests with attempts are archived instead,
// so results are never destroyed by a cleanup click.
func (r *Repo) DeleteAssessment(ctx context.Context, id string) error {
	var attempts int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM attempts WHERE assessment_id = $1`, id).Scan(&attempts); err != nil {
		return fmt.Errorf("count attempts: %w", err)
	}
	if attempts > 0 {
		_, err := r.pool.Exec(ctx, `UPDATE assessments SET status = 'archived', updated_at = now() WHERE id = $1`, id)
		if err != nil {
			return fmt.Errorf("archive assessment: %w", err)
		}
		return nil
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM assessments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete assessment: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertSection creates or updates one section of a test.
func (r *Repo) UpsertSection(ctx context.Context, req *assessmentv1.UpsertSectionRequest) (string, error) {
	s := req.Section
	if s == nil || s.AssessmentId == "" {
		return "", fmt.Errorf("assessment id is required")
	}
	if s.Kind != "mcq" && s.Kind != "coding" && s.Kind != "descriptive" {
		return "", fmt.Errorf("section kind must be mcq, coding or descriptive")
	}

	var pickCount any
	if s.PickCount > 0 {
		pickCount = s.PickCount
	}
	var duration any
	if s.DurationMinutes > 0 {
		duration = s.DurationMinutes
	}

	id := s.Id
	if id == "" {
		err := r.pool.QueryRow(ctx, `
			INSERT INTO assessment_sections (assessment_id, title, kind, order_index,
				duration_minutes, cutoff_marks, pick_count, pick_topic, pick_difficulty,
				pick_marks, partial_credit)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			RETURNING id
		`, s.AssessmentId, defaultStr(s.Title, "Section"), s.Kind, s.OrderIndex, duration,
			s.CutoffMarks, pickCount, s.PickTopic, s.PickDifficulty,
			maxInt32(s.PickMarks, 1), s.PartialCredit).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("insert section: %w", err)
		}
		return id, nil
	}

	ct, err := r.pool.Exec(ctx, `
		UPDATE assessment_sections SET
			title = $2, kind = $3, order_index = $4, duration_minutes = $5,
			cutoff_marks = $6, pick_count = $7, pick_topic = $8, pick_difficulty = $9,
			pick_marks = $10, partial_credit = $11
		WHERE id = $1
	`, id, defaultStr(s.Title, "Section"), s.Kind, s.OrderIndex, duration,
		s.CutoffMarks, pickCount, s.PickTopic, s.PickDifficulty,
		maxInt32(s.PickMarks, 1), s.PartialCredit)
	if err != nil {
		return "", fmt.Errorf("update section: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", ErrNotFound
	}
	return id, nil
}

// DeleteSection removes a section and its question list.
func (r *Repo) DeleteSection(ctx context.Context, id string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM assessment_sections WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete section: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSectionQuestions replaces a section's whole question list, which is how
// the builder saves a drag-reordered list in one round trip.
func (r *Repo) SetSectionQuestions(ctx context.Context, req *assessmentv1.SetSectionQuestionsRequest) error {
	var kind string
	err := r.pool.QueryRow(ctx, `SELECT kind FROM assessment_sections WHERE id = $1`, req.SectionId).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup section kind: %w", err)
	}

	// A coding section must hold problem references and an MCQ/descriptive
	// section must hold bank questions; mixing them would break both the paper
	// projection and scoring.
	for i, q := range req.Questions {
		switch kind {
		case "coding":
			if q.ProblemId == "" {
				return fmt.Errorf("question %d: a coding section needs a problem id", i+1)
			}
		default:
			if q.McqQuestionId == "" {
				return fmt.Errorf("question %d: a %s section needs a bank question id", i+1, kind)
			}
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin set section questions: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM section_questions WHERE section_id = $1`, req.SectionId); err != nil {
		return fmt.Errorf("clear section questions: %w", err)
	}
	for i, q := range req.Questions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO section_questions (section_id, mcq_question_id, problem_id, marks, order_index)
			VALUES ($1, $2, $3, $4, $5)
		`, req.SectionId, nullable(q.McqQuestionId), nullable(q.ProblemId), maxInt32(q.Marks, 1), i); err != nil {
			return fmt.Errorf("insert section question %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set section questions: %w", err)
	}
	return nil
}

func maxInt32(v, min int32) int32 {
	if v < min {
		return min
	}
	return v
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
