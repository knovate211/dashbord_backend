// Package sweeper closes attempts whose deadline has passed.
//
// The write paths already refuse work past expiry, but a candidate who simply
// closes the tab never makes another request — without this their attempt would
// sit `in_progress` forever and never appear in the recruiter's results.
package sweeper

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/knovate211/assessment-service/internal/repository"
)

// Run sweeps every interval until ctx is cancelled.
func Run(ctx context.Context, repo *repository.Repo, interval time.Duration, log *zap.Logger) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Info("attempt expiry sweeper started", zap.Duration("interval", interval))

	for {
		select {
		case <-ctx.Done():
			log.Info("attempt expiry sweeper stopped")
			return
		case <-ticker.C:
			sweepCtx, cancel := context.WithTimeout(ctx, interval)
			closed, err := repo.ExpireDueAttempts(sweepCtx)
			cancel()
			if err != nil {
				log.Error("expire due attempts", zap.Error(err))
				continue
			}
			if closed > 0 {
				log.Info("auto-submitted expired attempts", zap.Int("count", closed))
			}
		}
	}
}
