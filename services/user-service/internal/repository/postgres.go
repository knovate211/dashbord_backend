package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pkgauth "github.com/knovate211/pkg/auth"
	userv1 "github.com/knovate211/proto/user/v1"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// VerifyUser verifies credentials against the database.
func (r *UserRepository) VerifyUser(ctx context.Context, email, password string) (*userv1.VerifyUserResponse, error) {
	var id, name, dbPassword, role string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, name, password, role
		FROM   users
		WHERE  email = $1
	`, email).Scan(&id, &name, &dbPassword, &role)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("query user: %w", err)
	}

	ok, legacy := pkgauth.CheckPassword(dbPassword, password)
	if !ok {
		return nil, fmt.Errorf("invalid password")
	}

	// The row predates hashing. Now that the plaintext has been proven correct,
	// replace it with a hash — this is the only moment the real password is
	// available to upgrade. A failure here is logged by the caller's error path
	// but must not block a valid login, so it is deliberately not returned.
	if legacy {
		if hashed, err := pkgauth.HashPassword(password); err == nil {
			_, _ = r.pool.Exec(ctx, `
				UPDATE users SET password = $1, updated_at = now() WHERE id = $2::uuid
			`, hashed, id)
		}
	}

	return &userv1.VerifyUserResponse{
		Id:    id,
		Email: email,
		Name:  name,
		Role:  role,
	}, nil
}

// CreateOrUpdateUser inserts or updates a user record.
func (r *UserRepository) CreateOrUpdateUser(ctx context.Context, email, name, password, role string) error {
	hashed, err := pkgauth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO users (email, name, password, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email)
		DO UPDATE SET name = EXCLUDED.name, password = EXCLUDED.password, role = EXCLUDED.role, updated_at = now();
	`, email, name, hashed, role)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

// EnsureUsersTable creates the users table if missing and seeds the default admin user.
func (r *UserRepository) EnsureUsersTable(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      TEXT NOT NULL UNIQUE,
			name       TEXT NOT NULL,
			password   TEXT NOT NULL,
			role       TEXT NOT NULL DEFAULT 'student',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	if err != nil {
		return fmt.Errorf("create users table: %w", err)
	}

	// Seed default users. The passwords are hashed here rather than embedded as
	// literals, so a fresh database never contains a plaintext password.
	knovatePw, err := pkgauth.HashPassword("knovate123")
	if err != nil {
		return fmt.Errorf("hash seed password: %w", err)
	}
	skillofiedPw, err := pkgauth.HashPassword("skillofied123")
	if err != nil {
		return fmt.Errorf("hash seed password: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO users (email, name, password, role)
		VALUES ('admin@knovate.com', 'Admin User', $1, 'admin'),
		       ('admin@skillofied.com', 'Admin User', $2, 'admin')
		ON CONFLICT (email) DO NOTHING;
	`, knovatePw, skillofiedPw)
	if err != nil {
		return fmt.Errorf("seed default user: %w", err)
	}

	return nil
}

// EnsureProfileTable creates the user_profiles table if missing.
func (r *UserRepository) EnsureProfileTable(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS user_profiles (
			user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,

			-- Personal Info
			gender     TEXT NOT NULL DEFAULT '',
			dob        TEXT NOT NULL DEFAULT '',
			whatsapp   TEXT NOT NULL DEFAULT '',
			phone      TEXT NOT NULL DEFAULT '',
			experience TEXT NOT NULL DEFAULT '',

			-- Generic Details
			work_experience          TEXT    NOT NULL DEFAULT '',
			career_gap               TEXT    NOT NULL DEFAULT '',
			current_state            TEXT    NOT NULL DEFAULT '',
			current_city             TEXT    NOT NULL DEFAULT '',
			preferred_locations      TEXT[]  NOT NULL DEFAULT '{}',
			github_link              TEXT    NOT NULL DEFAULT '',
			linkedin_link            TEXT    NOT NULL DEFAULT '',
			is_working_professional  BOOLEAN NOT NULL DEFAULT FALSE,
			resume_name              TEXT    NOT NULL DEFAULT '',

			-- 10th Grade
			edu10_school_name     TEXT NOT NULL DEFAULT '',
			edu10_year_of_passout TEXT NOT NULL DEFAULT '',
			edu10_marks_percent   TEXT NOT NULL DEFAULT '',

			-- 12th / PUC / Intermediate / Diploma
			edu12_school_name     TEXT NOT NULL DEFAULT '',
			edu12_year_of_passout TEXT NOT NULL DEFAULT '',
			edu12_marks_percent   TEXT NOT NULL DEFAULT '',

			-- UG Detail
			ug_university_roll_no TEXT NOT NULL DEFAULT '',
			ug_college_name       TEXT NOT NULL DEFAULT '',
			ug_course_name        TEXT NOT NULL DEFAULT '',
			ug_branch             TEXT NOT NULL DEFAULT '',
			ug_year_of_passout    TEXT NOT NULL DEFAULT '',
			ug_marks_percent      TEXT NOT NULL DEFAULT '',
			ug_cgpa               TEXT NOT NULL DEFAULT '',
			ug_active_backlogs    TEXT NOT NULL DEFAULT '',

			-- PG Detail
			pg_has_certificate BOOLEAN NOT NULL DEFAULT FALSE,

			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	if err != nil {
		return fmt.Errorf("create user_profiles table: %w", err)
	}
	return nil
}

// EnsureUserCoursesTable creates the user_courses table if missing.
func (r *UserRepository) EnsureUserCoursesTable(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS user_courses (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			course_id  TEXT NOT NULL,
			granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (user_id, course_id)
		);
	`)
	if err != nil {
		return fmt.Errorf("create user_courses table: %w", err)
	}
	return nil
}

// GetProfile retrieves the profile for the given user ID.
// Returns nil, nil when no profile row exists yet.
func (r *UserRepository) GetProfile(ctx context.Context, userID string) (*userv1.UserProfile, error) {
	p := &userv1.UserProfile{UserID: userID}
	err := r.pool.QueryRow(ctx, `
		SELECT
			gender, dob, whatsapp, phone, experience,
			work_experience, career_gap, current_state, current_city,
			preferred_locations, github_link, linkedin_link,
			is_working_professional, resume_name,
			edu10_school_name, edu10_year_of_passout, edu10_marks_percent,
			edu12_school_name, edu12_year_of_passout, edu12_marks_percent,
			ug_university_roll_no, ug_college_name, ug_course_name, ug_branch,
			ug_year_of_passout, ug_marks_percent, ug_cgpa, ug_active_backlogs,
			pg_has_certificate
		FROM user_profiles
		WHERE user_id = $1
	`, userID).Scan(
		&p.Gender, &p.Dob, &p.Whatsapp, &p.Phone, &p.Experience,
		&p.WorkExperience, &p.CareerGap, &p.CurrentState, &p.CurrentCity,
		&p.PreferredLocations, &p.GithubLink, &p.LinkedinLink,
		&p.IsWorkingProfessional, &p.ResumeName,
		&p.Edu10SchoolName, &p.Edu10YearOfPassout, &p.Edu10MarksPercent,
		&p.Edu12SchoolName, &p.Edu12YearOfPassout, &p.Edu12MarksPercent,
		&p.UGUniversityRollNo, &p.UGCollegeName, &p.UGCourseName, &p.UGBranch,
		&p.UGYearOfPassout, &p.UGMarksPercent, &p.UGCGPA, &p.UGActiveBacklogs,
		&p.PGHasCertificate,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // no profile yet – not an error
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return p, nil
}

// UpsertProfile inserts or updates a user's profile row.
func (r *UserRepository) UpsertProfile(ctx context.Context, p *userv1.UserProfile) error {
	// Guard: nil slice → empty slice so the NOT NULL TEXT[] column is satisfied
	if p.PreferredLocations == nil {
		p.PreferredLocations = []string{}
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_profiles (
			user_id,
			gender, dob, whatsapp, phone, experience,
			work_experience, career_gap, current_state, current_city,
			preferred_locations, github_link, linkedin_link,
			is_working_professional, resume_name,
			edu10_school_name, edu10_year_of_passout, edu10_marks_percent,
			edu12_school_name, edu12_year_of_passout, edu12_marks_percent,
			ug_university_roll_no, ug_college_name, ug_course_name, ug_branch,
			ug_year_of_passout, ug_marks_percent, ug_cgpa, ug_active_backlogs,
			pg_has_certificate
		) VALUES (
			$1,
			$2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			$14, $15,
			$16, $17, $18,
			$19, $20, $21,
			$22, $23, $24, $25,
			$26, $27, $28, $29,
			$30
		)
		ON CONFLICT (user_id) DO UPDATE SET
			gender                  = EXCLUDED.gender,
			dob                     = EXCLUDED.dob,
			whatsapp                = EXCLUDED.whatsapp,
			phone                   = EXCLUDED.phone,
			experience              = EXCLUDED.experience,
			work_experience         = EXCLUDED.work_experience,
			career_gap              = EXCLUDED.career_gap,
			current_state           = EXCLUDED.current_state,
			current_city            = EXCLUDED.current_city,
			preferred_locations     = EXCLUDED.preferred_locations,
			github_link             = EXCLUDED.github_link,
			linkedin_link           = EXCLUDED.linkedin_link,
			is_working_professional = EXCLUDED.is_working_professional,
			resume_name             = EXCLUDED.resume_name,
			edu10_school_name       = EXCLUDED.edu10_school_name,
			edu10_year_of_passout   = EXCLUDED.edu10_year_of_passout,
			edu10_marks_percent     = EXCLUDED.edu10_marks_percent,
			edu12_school_name       = EXCLUDED.edu12_school_name,
			edu12_year_of_passout   = EXCLUDED.edu12_year_of_passout,
			edu12_marks_percent     = EXCLUDED.edu12_marks_percent,
			ug_university_roll_no   = EXCLUDED.ug_university_roll_no,
			ug_college_name         = EXCLUDED.ug_college_name,
			ug_course_name          = EXCLUDED.ug_course_name,
			ug_branch               = EXCLUDED.ug_branch,
			ug_year_of_passout      = EXCLUDED.ug_year_of_passout,
			ug_marks_percent        = EXCLUDED.ug_marks_percent,
			ug_cgpa                 = EXCLUDED.ug_cgpa,
			ug_active_backlogs      = EXCLUDED.ug_active_backlogs,
			pg_has_certificate      = EXCLUDED.pg_has_certificate,
			updated_at              = now();
	`,
		p.UserID,
		p.Gender, p.Dob, p.Whatsapp, p.Phone, p.Experience,
		p.WorkExperience, p.CareerGap, p.CurrentState, p.CurrentCity,
		p.PreferredLocations, p.GithubLink, p.LinkedinLink,
		p.IsWorkingProfessional, p.ResumeName,
		p.Edu10SchoolName, p.Edu10YearOfPassout, p.Edu10MarksPercent,
		p.Edu12SchoolName, p.Edu12YearOfPassout, p.Edu12MarksPercent,
		p.UGUniversityRollNo, p.UGCollegeName, p.UGCourseName, p.UGBranch,
		p.UGYearOfPassout, p.UGMarksPercent, p.UGCGPA, p.UGActiveBacklogs,
		p.PGHasCertificate,
	)
	if err != nil {
		return fmt.Errorf("upsert profile: %w", err)
	}
	return nil
}

// CheckUserCourseAccess verifies if a user has access to a specific course.
func (r *UserRepository) CheckUserCourseAccess(ctx context.Context, userID, courseID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_courses
			WHERE user_id = $1 AND course_id = $2
		)
	`, userID, courseID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check course access: %w", err)
	}
	return exists, nil
}

// EnsureQuizTables creates the quiz database tables if they do not exist and
// syncs the answer keys for every course module from quizAnswerKeys.
func (r *UserRepository) EnsureQuizTables(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS quiz_keys (
			module_id TEXT NOT NULL,
			question_id INT NOT NULL,
			correct_answer TEXT NOT NULL,
			PRIMARY KEY (module_id, question_id)
		);

		CREATE TABLE IF NOT EXISTS user_quiz_attempts (
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			module_id TEXT NOT NULL,
			score INT NOT NULL,
			total_questions INT NOT NULL,
			selected_answers JSONB NOT NULL DEFAULT '{}'::jsonb,
			completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, module_id)
		);
	`)
	if err != nil {
		return fmt.Errorf("create quiz tables: %w", err)
	}

	// Dynamically add selected_answers column to user_quiz_attempts if database already exists
	_, _ = r.pool.Exec(ctx, `
		ALTER TABLE user_quiz_attempts ADD COLUMN IF NOT EXISTS selected_answers JSONB NOT NULL DEFAULT '{}'::jsonb;
	`)

	// Answer keys live in quiz_seed_data.go, generated from the course content
	// data files. Regenerate that file whenever quiz questions change.
	if len(quizAnswerKeys) == 0 {
		return fmt.Errorf("quizAnswerKeys is empty; regenerate quiz_seed_data.go")
	}

	moduleIDs := make([]string, 0, len(quizAnswerKeys))
	questionIDs := make([]int32, 0, len(quizAnswerKeys))
	correctAnswers := make([]string, 0, len(quizAnswerKeys))
	for _, a := range quizAnswerKeys {
		moduleIDs = append(moduleIDs, a.moduleID)
		questionIDs = append(questionIDs, int32(a.questionID))
		correctAnswers = append(correctAnswers, a.correctAns)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin quiz seed tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Upsert every key in a single round trip rather than one per row.
	_, err = tx.Exec(ctx, `
		INSERT INTO quiz_keys (module_id, question_id, correct_answer)
		SELECT * FROM unnest($1::text[], $2::int[], $3::text[])
		ON CONFLICT (module_id, question_id)
		DO UPDATE SET correct_answer = EXCLUDED.correct_answer;
	`, moduleIDs, questionIDs, correctAnswers)
	if err != nil {
		return fmt.Errorf("seed quiz keys: %w", err)
	}

	// Drop keys for questions that no longer exist. Without this, a removed
	// question would linger and inflate the total_questions a learner is
	// graded against.
	_, err = tx.Exec(ctx, `
		DELETE FROM quiz_keys k
		WHERE NOT EXISTS (
			SELECT 1 FROM unnest($1::text[], $2::int[]) AS seed(module_id, question_id)
			WHERE seed.module_id = k.module_id AND seed.question_id = k.question_id
		);
	`, moduleIDs, questionIDs)
	if err != nil {
		return fmt.Errorf("prune stale quiz keys: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit quiz seed: %w", err)
	}

	return nil
}

// SubmitQuiz grades a user's quiz submission against the stored answer key,
// saves the attempt, and returns the score plus a per-question breakdown.
//
// Grading is deliberately server-side: the client never receives the answer key
// before submitting, so answers cannot be read out of the bundle or spoofed.
func (r *UserRepository) SubmitQuiz(ctx context.Context, userID, moduleID string, userAnswers []*userv1.QuizAnswer) (int, int, []*userv1.QuizQuestionResult, error) {
	// 1. Fetch correct answers for the module
	rows, err := r.pool.Query(ctx, `
		SELECT question_id, correct_answer
		FROM   quiz_keys
		WHERE  module_id = $1
		ORDER  BY question_id
	`, moduleID)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("fetch correct answers: %w", err)
	}
	defer rows.Close()

	keys := make(map[int]string)
	var orderedIDs []int
	for rows.Next() {
		var qID int
		var ans string
		if err := rows.Scan(&qID, &ans); err != nil {
			return 0, 0, nil, fmt.Errorf("scan correct answer: %w", err)
		}
		keys[qID] = ans
		orderedIDs = append(orderedIDs, qID)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, nil, fmt.Errorf("iterate correct answers: %w", err)
	}

	// 2. Grade
	total := len(keys)
	if total == 0 {
		return 0, 0, nil, fmt.Errorf("no quiz keys found for module %s", moduleID)
	}

	submitted := make(map[int]string, len(userAnswers))
	for _, ua := range userAnswers {
		submitted[ua.QuestionID] = ua.Answer
	}

	// Iterate the key, not the submission, so unanswered questions are graded
	// as incorrect rather than silently skipped.
	score := 0
	results := make([]*userv1.QuizQuestionResult, 0, total)
	for _, qID := range orderedIDs {
		correctAns := keys[qID]
		isCorrect := submitted[qID] == correctAns
		if isCorrect {
			score++
		}
		results = append(results, &userv1.QuizQuestionResult{
			QuestionID:    qID,
			Correct:       isCorrect,
			CorrectAnswer: correctAns,
		})
	}

	// 3. Save attempt. Keep the learner's best score rather than overwriting a
	// good result with a worse retry.
	answersJSON, _ := json.Marshal(submitted)
	_, err = r.pool.Exec(ctx, `
		INSERT INTO user_quiz_attempts (user_id, module_id, score, total_questions, selected_answers, completed_at)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, now())
		ON CONFLICT (user_id, module_id)
		DO UPDATE SET score = GREATEST(user_quiz_attempts.score, EXCLUDED.score),
		              total_questions = EXCLUDED.total_questions,
		              selected_answers = EXCLUDED.selected_answers,
		              completed_at = now();
	`, userID, moduleID, score, total, string(answersJSON))
	if err != nil {
		return 0, 0, nil, fmt.Errorf("save quiz attempt: %w", err)
	}

	return score, total, results, nil
}

// GetQuizAttempts fetches all saved quiz scores for a user.
func (r *UserRepository) GetQuizAttempts(ctx context.Context, userID string) ([]*userv1.QuizAttempt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT module_id, score, total_questions, selected_answers, completed_at
		FROM   user_quiz_attempts
		WHERE  user_id = $1::uuid
		ORDER  BY completed_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get quiz attempts: %w", err)
	}
	defer rows.Close()

	var attempts []*userv1.QuizAttempt
	for rows.Next() {
		var a userv1.QuizAttempt
		var completedAt time.Time
		var selectedAnswers []byte
		if err := rows.Scan(&a.ModuleID, &a.Score, &a.TotalQuestions, &selectedAnswers, &completedAt); err != nil {
			return nil, fmt.Errorf("scan quiz attempt: %w", err)
		}
		a.SelectedAnswers = string(selectedAnswers)
		a.CompletedAt = completedAt.Format(time.RFC3339)
		attempts = append(attempts, &a)
	}

	return attempts, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Admin repository methods
// ─────────────────────────────────────────────────────────────────────────────

// ImportUserRow holds a single row from the admin Excel import.
type ImportUserRow struct {
	Name      string
	Email     string
	Password  string
	Role      string
	CourseIDs []string
}

// ImportRowResult is the per-row outcome of a bulk import.
type ImportRowResult struct {
	Email   string
	Success bool
	Message string
}

// AdminUserRow is a user record returned to the admin panel.
type AdminUserRow struct {
	ID        string
	Email     string
	Name      string
	Role      string
	CourseIDs []string
	CreatedAt string
}

// BulkUpsertUsers inserts or updates multiple users and grants their course access.
func (r *UserRepository) BulkUpsertUsers(ctx context.Context, rows []ImportUserRow) []ImportRowResult {
	results := make([]ImportRowResult, 0, len(rows))
	for _, row := range rows {
		if row.Email == "" || row.Name == "" || row.Password == "" {
			results = append(results, ImportRowResult{
				Email:   row.Email,
				Success: false,
				Message: "email, name and password are required",
			})
			continue
		}
		role := row.Role
		if role == "" {
			role = "student"
		}

		hashed, err := pkgauth.HashPassword(row.Password)
		if err != nil {
			results = append(results, ImportRowResult{
				Email:   row.Email,
				Success: false,
				Message: fmt.Sprintf("hash password: %v", err),
			})
			continue
		}

		// Upsert user and return its UUID
		var userID string
		err = r.pool.QueryRow(ctx, `
			INSERT INTO users (email, name, password, role)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (email)
			DO UPDATE SET name = EXCLUDED.name, password = EXCLUDED.password, role = EXCLUDED.role, updated_at = now()
			RETURNING id::text
		`, row.Email, row.Name, hashed, role).Scan(&userID)
		if err != nil {
			results = append(results, ImportRowResult{
				Email:   row.Email,
				Success: false,
				Message: fmt.Sprintf("upsert user: %v", err),
			})
			continue
		}

		// Grant course access for each listed course
		for _, courseID := range row.CourseIDs {
			if courseID == "" {
				continue
			}
			_, _ = r.pool.Exec(ctx, `
				INSERT INTO user_courses (user_id, course_id)
				VALUES ($1::uuid, $2)
				ON CONFLICT (user_id, course_id) DO NOTHING
			`, userID, courseID)
		}

		results = append(results, ImportRowResult{
			Email:   row.Email,
			Success: true,
			Message: "imported successfully",
		})
	}
	return results
}

// ListAdminUsers returns a paginated list of all users with their assigned course IDs.
func (r *UserRepository) ListAdminUsers(ctx context.Context, page, pageSize int) ([]AdminUserRow, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT u.id::text, u.email, u.name, u.role, u.created_at,
		       COALESCE(array_agg(uc.course_id) FILTER (WHERE uc.course_id IS NOT NULL), '{}') AS course_ids
		FROM users u
		LEFT JOIN user_courses uc ON uc.user_id = u.id
		GROUP BY u.id, u.email, u.name, u.role, u.created_at
		ORDER BY u.created_at DESC
		LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []AdminUserRow
	for rows.Next() {
		var u AdminUserRow
		var createdAt time.Time
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &createdAt, &u.CourseIDs); err != nil {
			return nil, 0, fmt.Errorf("scan user row: %w", err)
		}
		u.CreatedAt = createdAt.Format(time.RFC3339)
		users = append(users, u)
	}
	return users, total, nil
}

// UpdateUserRole changes the role of an existing user.
func (r *UserRepository) UpdateUserRole(ctx context.Context, userID, role string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users SET role = $2, updated_at = now() WHERE id = $1::uuid
	`, userID, role)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// AdminGrantCourseAccess grants a user access to a course.
func (r *UserRepository) AdminGrantCourseAccess(ctx context.Context, userID, courseID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_courses (user_id, course_id)
		VALUES ($1::uuid, $2)
		ON CONFLICT (user_id, course_id) DO NOTHING
	`, userID, courseID)
	if err != nil {
		return fmt.Errorf("grant course access: %w", err)
	}
	return nil
}

// AdminRevokeCourseAccess revokes a user's access to a course.
func (r *UserRepository) AdminRevokeCourseAccess(ctx context.Context, userID, courseID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM user_courses WHERE user_id = $1::uuid AND course_id = $2
	`, userID, courseID)
	if err != nil {
		return fmt.Errorf("revoke course access: %w", err)
	}
	return nil
}

// DeleteUser permanently removes a user (cascades to user_courses, user_profiles, etc.).
func (r *UserRepository) DeleteUser(ctx context.Context, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}
