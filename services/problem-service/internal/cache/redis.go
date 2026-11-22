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

// InvalidateProblem drops every cached view that can contain a problem: its
// detail (cached under both id and slug) and every problem list (titles and
// difficulty appear there). Called after an admin creates, edits or deletes one.
//
// Practice-set summaries are deliberately not cached: they carry each
// learner's progress, which changes in another service when they solve a
// problem, and the query behind them runs in about a millisecond.
//
// List keys embed their filters, so they are found with SCAN; DEL does not
// expand a glob, which is why the earlier "problems:list:*" never matched.
func (c *ProblemCache) InvalidateProblem(ctx context.Context, id, slug string) {
	if !c.available() {
		return
	}
	keys := []string{problemKey(id)}
	if slug != "" {
		keys = append(keys, problemKey(slug))
		}
	iter := c.client.Scan(ctx, 0, "problems:list:*", 200).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		c.log.Warn("cache invalidation scan failed", zap.Error(err))
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		c.log.Warn("cache invalidation failed", zap.Strings("keys", keys), zap.Error(err))
	}
}
