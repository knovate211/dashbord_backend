// Package cache provides Redis-backed caching for the progress service.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	progressv1 "github.com/knovate211/proto/progress/v1"
)

const (
	ttlUserProgress   = 3 * time.Minute
	ttlProblemStatus  = 1 * time.Minute
)

// ProgressCache wraps a Redis client for the progress service.
type ProgressCache struct {
	client *redis.Client
}

// New constructs a ProgressCache. nil client means no-op caching.
func New(client *redis.Client) *ProgressCache {
	return &ProgressCache{client: client}
}

func (c *ProgressCache) available() bool { return c.client != nil }

// GetUserProgress retrieves cached user progress.
func (c *ProgressCache) GetUserProgress(ctx context.Context, userID string) (*progressv1.UserProgress, error) {
	if !c.available() {
		return nil, fmt.Errorf("cache unavailable")
	}
	data, err := c.client.Get(ctx, "progress:user:"+userID).Bytes()
	if err != nil {
		return nil, err
	}
	var p progressv1.UserProgress
	return &p, json.Unmarshal(data, &p)
}

// SetUserProgress caches user progress.
func (c *ProgressCache) SetUserProgress(ctx context.Context, p *progressv1.UserProgress) error {
	if !c.available() {
		return nil
	}
	data, _ := json.Marshal(p)
	return c.client.Set(ctx, "progress:user:"+p.UserId, data, ttlUserProgress).Err()
}

// InvalidateUserProgress removes cached user progress (call on update).
func (c *ProgressCache) InvalidateUserProgress(ctx context.Context, userID string) {
	if !c.available() {
		return
	}
	c.client.Del(ctx, "progress:user:"+userID) //nolint:errcheck
}

// GetProblemStatus retrieves cached problem status.
func (c *ProgressCache) GetProblemStatus(ctx context.Context, userID, problemID string) (*progressv1.ProblemStatus, error) {
	if !c.available() {
		return nil, fmt.Errorf("cache unavailable")
	}
	data, err := c.client.Get(ctx, fmt.Sprintf("progress:problem:%s:%s", userID, problemID)).Bytes()
	if err != nil {
		return nil, err
	}
	var s progressv1.ProblemStatus
	return &s, json.Unmarshal(data, &s)
}

// SetProblemStatus caches problem status.
func (c *ProgressCache) SetProblemStatus(ctx context.Context, s *progressv1.ProblemStatus) error {
	if !c.available() {
		return nil
	}
	data, _ := json.Marshal(s)
	key := fmt.Sprintf("progress:problem:%s:%s", s.UserId, s.ProblemId)
	return c.client.Set(ctx, key, data, ttlProblemStatus).Err()
}
