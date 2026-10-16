// Package repository is the PostgreSQL persistence layer for the assessment
// service.
//
// Every service in this workspace shares one Postgres database, so a few
// read-only joins reach into tables owned elsewhere — `users` for candidate
// name/email on recruiter reports, `problems` for coding question titles.
// Writes stay strictly inside the tables this service owns.
package repository

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// Repo holds the connection pool. All methods are safe for concurrent use.
type Repo struct {
	pool *pgxpool.Pool
}

// New constructs a Repo.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Pool exposes the underlying pool for components that run their own queries
// (the graded-submission consumer and the expiry sweeper).
func (r *Repo) Pool() *pgxpool.Pool { return r.pool }

// ─── shared helpers ───────────────────────────────────────────────────────────

// nullable turns an empty string into a SQL NULL, which is what UUID and
// nullable text columns expect rather than ”.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// parseTime accepts RFC3339 (what the API speaks) and returns nil for empty
// input so optional timestamps land as NULL.
func parseTime(s string) any {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return t
}

// fmtTime renders a nullable timestamp as RFC3339, or "" when absent.
func fmtTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func toJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal jsonb: %w", err)
	}
	return b, nil
}

func fromJSON(raw []byte, out any) {
	if len(raw) == 0 {
		return
	}
	_ = json.Unmarshal(raw, out) // a malformed blob degrades to the zero value
}

// clampPage normalizes user-supplied pagination.
func clampPage(page, pageSize int32) (int32, int32, int32) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	return page, pageSize, (page - 1) * pageSize
}

// defaultProctoring is applied when an assessment stores an empty config.
func defaultProctoring() *assessmentv1.Proctoring {
	return &assessmentv1.Proctoring{}
}
