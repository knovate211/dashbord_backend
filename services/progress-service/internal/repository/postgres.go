// Package repository provides Postgres-backed data access for the progress service.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	progressv1 "github.com/knovate211/proto/progress/v1"
)

// ProgressRepository wraps a pgxpool for progress CRUD.
type ProgressRepository struct {
	pool *pgxpool.Pool
}

// New constructs a ProgressRepository.
func New(pool *pgxpool.Pool) *ProgressRepository {
	return &ProgressRepository{pool: pool}
}

// GetUserProgress returns the aggregate progress summary for a user.
func (r *ProgressRepository) GetUserProgress(ctx context.Context, req *progressv1.GetUserProgressRequest) (*progressv1.UserProgress, error) {
	up := &progressv1.UserProgress{UserId: req.UserId}

	err := r.pool.QueryRow(ctx, `
		SELECT total_solved, total_attempted, easy_solved, medium_solved, hard_solved,
		       current_streak, longest_streak, total_xp
		FROM   user_progress
		WHERE  user_id = $1
	`, req.UserId).Scan(
		&up.TotalSolved, &up.TotalAttempted,
		&up.EasySolved, &up.MediumSolved, &up.HardSolved,
		&up.CurrentStreak, &up.LongestStreak, &up.TotalXp,
	)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get user progress: %w", err)
	}

	// Per-set progress
	rows, err := r.pool.Query(ctx, `
		SELECT sp.set_id::text, COALESCE(ps.title, ''), sp.solved, sp.total
		FROM   set_progress sp
		LEFT   JOIN practice_sets ps ON ps.id = sp.set_id
		WHERE  sp.user_id = $1
	`, req.UserId)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			sp := &progressv1.SetProgress{}
			rows.Scan(&sp.SetId, &sp.Title, &sp.Solved, &sp.Total) //nolint:errcheck
			if sp.Total > 0 {
				sp.Progress = float32(sp.Solved) / float32(sp.Total) * 100
			}
			up.SetProgress = append(up.SetProgress, sp)
		}
	}

	return up, nil
}

// GetProblemStatus returns a user's status for a specific problem.
func (r *ProgressRepository) GetProblemStatus(ctx context.Context, req *progressv1.GetProblemStatusRequest) (*progressv1.ProblemStatus, error) {
	ps := &progressv1.ProblemStatus{
		UserId:    req.UserId,
		ProblemId: req.ProblemId,
		Status:    "Unsolved",
	}

	var solvedAt *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT status, solved_at, attempts
		FROM   problem_progress
		WHERE  user_id = $1 AND problem_id::text = $2
	`, req.UserId, req.ProblemId).Scan(&ps.Status, &solvedAt, &ps.Attempts)

	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get problem status: %w", err)
	}
	if solvedAt != nil {
		ps.SolvedAt = solvedAt.UTC().Format(time.RFC3339)
	}

	return ps, nil
}

// UpdateProblemStatus updates a user's problem status and aggregate progress.
func (r *ProgressRepository) UpdateProblemStatus(ctx context.Context, req *progressv1.UpdateProblemStatusRequest) (*progressv1.UpdateProblemStatusResponse, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	now := time.Now().UTC()

	// Check existing problem progress to check already solved/attempted
	var currentStatus string
	var alreadySolved bool
	var alreadyAttempted bool

	err = tx.QueryRow(ctx, `
		SELECT status FROM problem_progress
		WHERE  user_id = $1 AND problem_id::text = $2
	`, req.UserId, req.ProblemId).Scan(&currentStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			alreadySolved = false
			alreadyAttempted = false
		} else {
			return nil, fmt.Errorf("check existing problem progress: %w", err)
		}
	} else {
		alreadyAttempted = true
		alreadySolved = (currentStatus == "Solved")
	}

	// Fetch problem metadata: difficulty, set_id, xp
	var difficulty string
	var setID *string
	var problemXP int32
	err = tx.QueryRow(ctx, `
		SELECT difficulty, set_id::text, xp
		FROM   problems
		WHERE  id::text = $1
	`, req.ProblemId).Scan(&difficulty, &setID, &problemXP)
	if err != nil {
		return nil, fmt.Errorf("get problem details: %w", err)
	}

	var setIDStr string
	if setID != nil {
		setIDStr = *setID
	}

	isNewSolve := req.IsCorrect && !alreadySolved
	isNewAttempt := !alreadyAttempted

	xpEarned := int32(0)
	if isNewSolve {
		if req.XpEarned > 0 {
			xpEarned = req.XpEarned
		} else {
			xpEarned = problemXP
		}
	}

	var solvedAt interface{}
	if req.IsCorrect || alreadySolved {
		solvedAt = now
	}

	statusToStore := req.Status
	if alreadySolved {
		statusToStore = "Solved"
	}

	// Upsert problem_progress
	_, err = tx.Exec(ctx, `
		INSERT INTO problem_progress (user_id, problem_id, set_id, status, solved_at, attempts, runtime_ms, memory_kb, language, updated_at)
		VALUES ($1, $2, NULLIF($3,'')::uuid, $4, $5, 1, $6, $7, $8, $9)
		ON CONFLICT (user_id, problem_id) DO UPDATE SET
		    status     = CASE WHEN problem_progress.status = 'Solved' THEN 'Solved' ELSE EXCLUDED.status END,
		    solved_at  = CASE WHEN problem_progress.solved_at IS NOT NULL THEN problem_progress.solved_at ELSE EXCLUDED.solved_at END,
		    attempts   = problem_progress.attempts + 1,
		    runtime_ms = CASE WHEN EXCLUDED.status = 'Solved' THEN EXCLUDED.runtime_ms ELSE problem_progress.runtime_ms END,
		    memory_kb  = CASE WHEN EXCLUDED.status = 'Solved' THEN EXCLUDED.memory_kb ELSE problem_progress.memory_kb END,
		    language   = EXCLUDED.language,
		    updated_at = EXCLUDED.updated_at
	`, req.UserId, req.ProblemId, setIDStr, statusToStore, solvedAt,
		req.RuntimeMs, req.MemoryKb, req.Language, now)
	if err != nil {
		return nil, fmt.Errorf("upsert problem progress: %w", err)
	}

	// Upsert problem_user_status (for fast problem status queries)
	_, err = tx.Exec(ctx, `
		INSERT INTO problem_user_status (user_id, problem_id, status, solved_at, attempts)
		VALUES ($1, $2, $3, $4, 1)
		ON CONFLICT (user_id, problem_id) DO UPDATE SET
		    status    = CASE WHEN problem_user_status.status = 'Solved' THEN 'Solved' ELSE EXCLUDED.status END,
		    solved_at = CASE WHEN problem_user_status.solved_at IS NOT NULL THEN problem_user_status.solved_at ELSE EXCLUDED.solved_at END,
		    attempts  = problem_user_status.attempts + 1
	`, req.UserId, req.ProblemId, statusToStore, solvedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert problem user status: %w", err)
	}

	// Upsert user_progress aggregate
	_, err = tx.Exec(ctx, `
		INSERT INTO user_progress (user_id, total_solved, total_attempted, easy_solved, medium_solved, hard_solved, total_xp, last_active_date, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id) DO UPDATE SET
		    total_solved    = user_progress.total_solved    + $2,
		    total_attempted = user_progress.total_attempted + $3,
		    easy_solved     = user_progress.easy_solved     + $4,
		    medium_solved   = user_progress.medium_solved   + $5,
		    hard_solved     = user_progress.hard_solved     + $6,
		    total_xp        = user_progress.total_xp        + $7,
		    last_active_date= $8,
		    updated_at      = $9
	`,
		req.UserId,
		boolToInt(isNewSolve),
		boolToInt(isNewAttempt),
		boolToInt(isNewSolve && difficulty == "Easy"),
		boolToInt(isNewSolve && difficulty == "Medium"),
		boolToInt(isNewSolve && difficulty == "Hard"),
		xpEarned,
		now.Format("2006-01-02"),
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert user progress: %w", err)
	}

	// Upsert set_progress if problem belongs to a practice set
	if setIDStr != "" {
		var totalProblems int32
		err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM problems WHERE set_id::text = $1", setIDStr).Scan(&totalProblems)
		if err != nil {
			return nil, fmt.Errorf("count problems in set: %w", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO set_progress (user_id, set_id, solved, total, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (user_id, set_id) DO UPDATE SET
			    solved     = set_progress.solved + $3,
			    total      = $4,
			    updated_at = $5
		`, req.UserId, setIDStr, boolToInt(isNewSolve), totalProblems, now)
		if err != nil {
			return nil, fmt.Errorf("upsert set progress: %w", err)
		}
	}

	// Log activity for streak tracking
	_, _ = tx.Exec(ctx, `
		INSERT INTO activity_log (user_id, activity_date)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, req.UserId, now.Format("2006-01-02"))

	// Recalculate streak
	streak := calculateStreak(ctx, tx, req.UserId)
	_, _ = tx.Exec(ctx, `
		UPDATE user_progress
		SET    current_streak = $1,
		       longest_streak = GREATEST(longest_streak, $1)
		WHERE  user_id = $2
	`, streak, req.UserId)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Read updated XP
	var newXp int32
	r.pool.QueryRow(ctx, "SELECT total_xp FROM user_progress WHERE user_id = $1", req.UserId).Scan(&newXp) //nolint:errcheck

	return &progressv1.UpdateProblemStatusResponse{
		Success:    true,
		NewTotalXp: newXp,
	}, nil
}

// getDifficulty returns the difficulty level of a problem from the DB.
func getDifficulty(ctx context.Context, tx pgx.Tx, problemId string) string {
	var d string
	tx.QueryRow(ctx, "SELECT difficulty FROM problems WHERE id::text = $1", problemId).Scan(&d) //nolint:errcheck
	return d
}

// calculateStreak counts consecutive active days ending today.
func calculateStreak(ctx context.Context, tx pgx.Tx, userID string) int32 {
	rows, err := tx.Query(ctx, `
		SELECT activity_date FROM activity_log
		WHERE  user_id = $1
		ORDER  BY activity_date DESC
	`, userID)
	if err != nil {
		return 0
	}
	defer rows.Close()

	var streak int32
	expected := time.Now().UTC().Truncate(24 * time.Hour)
	for rows.Next() {
		var d time.Time
		rows.Scan(&d) //nolint:errcheck
		d = d.Truncate(24 * time.Hour)
		if d.Equal(expected) || d.Equal(expected.Add(-24*time.Hour)) {
			streak++
			expected = d.Add(-24 * time.Hour)
		} else {
			break
		}
	}
	return streak
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
