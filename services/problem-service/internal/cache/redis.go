// Package cache provides Redis-backed caching for the problem service.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	problemv1 "github.com/knovate211/proto/problem/v1"
)

const (
	ttlProblemList     = 5 * time.Minute
	ttlProblemDetail   = 15 * time.Minute
	ttlPracticeSets    = 10 * time.Minute
	ttlUserStatus      = 2 * time.Minute
)

// ProblemCache wraps a Redis client with type-safe methods for the problem service.
type ProblemCache struct {
	client *redis.Client
	log    *zap.Logger
}

// New constructs a ProblemCache. If client is nil, all methods are no-ops.
func New(client *redis.Client, log *zap.Logger) *ProblemCache {
	return &ProblemCache{client: client, log: log}
}

func (c *ProblemCache) available() bool {
	return c.client != nil
}

// ── List Problems ────────────────────────────────────────────────────────────

func listKey(req *problemv1.ListProblemsRequest) string {
	return fmt.Sprintf("problems:list:%s:%s:%s:%d:%d:%s",
		req.SetId, req.Topic, req.Difficulty, req.Page, req.PageSize, req.UserId)
}

func (c *ProblemCache) GetListProblems(ctx context.Context, req *problemv1.ListProblemsRequest) (*problemv1.ListProblemsResponse, error) {
	if !c.available() {
		return nil, fmt.Errorf("cache unavailable")
	}
	data, err := c.client.Get(ctx, listKey(req)).Bytes()
	if err != nil {
		return nil, err
	}
	var resp problemv1.ListProblemsResponse
	return &resp, json.Unmarshal(data, &resp)
}

func (c *ProblemCache) SetListProblems(ctx context.Context, req *problemv1.ListProblemsRequest, resp *problemv1.ListProblemsResponse) error {
	if !c.available() {
		return nil
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, listKey(req), data, ttlProblemList).Err()
}

// ── Single Problem ────────────────────────────────────────────────────────────

func problemKey(id string) string { return "problems:detail:" + id }

func (c *ProblemCache) GetProblem(ctx context.Context, id string) (*problemv1.Problem, error) {
	if !c.available() {
		return nil, fmt.Errorf("cache unavailable")
	}
	data, err := c.client.Get(ctx, problemKey(id)).Bytes()
	if err != nil {
		return nil, err
	}
	var p problemv1.Problem
	return &p, json.Unmarshal(data, &p)
}

func (c *ProblemCache) SetProblem(ctx context.Context, p *problemv1.Problem) error {
	if !c.available() {
		return nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	// Cache by both ID and slug for dual-key lookups
	pipe := c.client.Pipeline()
	pipe.Set(ctx, problemKey(p.Id), data, ttlProblemDetail)
	pipe.Set(ctx, problemKey(p.Slug), data, ttlProblemDetail)
	_, err = pipe.Exec(ctx)
	return err
}

// ── Practice Sets ─────────────────────────────────────────────────────────────

func practiceSetKey(userID string) string {
	if userID == "" {
		return "practice_sets:all"
	}
	return "practice_sets:user:" + userID
}

func (c *ProblemCache) GetPracticeSets(ctx context.Context, userID string) (*problemv1.ListPracticeSetsResponse, error) {
	if !c.available() {
		return nil, fmt.Errorf("cache unavailable")
	}
	data, err := c.client.Get(ctx, practiceSetKey(userID)).Bytes()
	if err != nil {
		return nil, err
	}
	var resp problemv1.ListPracticeSetsResponse
	return &resp, json.Unmarshal(data, &resp)
}

func (c *ProblemCache) SetPracticeSets(ctx context.Context, userID string, resp *problemv1.ListPracticeSetsResponse) error {
	if !c.available() {
		return nil
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, practiceSetKey(userID), data, ttlPracticeSets).Err()
}

// ── User Status ───────────────────────────────────────────────────────────────

func userStatusKey(userID, problemID string) string {
	return fmt.Sprintf("user_status:%s:%s", userID, problemID)
}

func (c *ProblemCache) GetUserStatus(ctx context.Context, userID, problemID string) (*problemv1.ProblemUserStatus, error) {
	if !c.available() {
		return nil, fmt.Errorf("cache unavailable")
	}
	data, err := c.client.Get(ctx, userStatusKey(userID, problemID)).Bytes()
	if err != nil {
		return nil, err
	}
	var s problemv1.ProblemUserStatus
	return &s, json.Unmarshal(data, &s)
}

func (c *ProblemCache) SetUserStatus(ctx context.Context, s *problemv1.ProblemUserStatus) error {
	if !c.available() {
		return nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, userStatusKey(s.UserId, s.ProblemId), data, ttlUserStatus).Err()
}

// InvalidateProblem removes cached entries for a problem (call on update).
func (c *ProblemCache) InvalidateProblem(ctx context.Context, id, slug string) {
	if !c.available() {
		return
	}
	keys := []string{problemKey(id), problemKey(slug), "problems:list:*"}
	for _, k := range keys {
		if err := c.client.Del(ctx, k).Err(); err != nil {
			c.log.Warn("cache invalidation failed", zap.String("key", k), zap.Error(err))
		}
	}
}
