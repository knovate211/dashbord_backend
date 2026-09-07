// Package handler implements the gRPC ProgressService server.
package handler

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/knovate211/progress-service/internal/cache"
	"github.com/knovate211/progress-service/internal/repository"
	progressv1 "github.com/knovate211/proto/progress/v1"
)

// ProgressHandler implements progressv1.ProgressServiceServer.
type ProgressHandler struct {
	progressv1.UnimplementedProgressServiceServer
	repo   *repository.ProgressRepository
	cache  *cache.ProgressCache
	logger *zap.Logger
}

// New constructs a ProgressHandler.
func New(repo *repository.ProgressRepository, c *cache.ProgressCache, log *zap.Logger) *ProgressHandler {
	return &ProgressHandler{repo: repo, cache: c, logger: log}
}

// GetUserProgress returns aggregate progress for a user.
func (h *ProgressHandler) GetUserProgress(ctx context.Context, req *progressv1.GetUserProgressRequest) (*progressv1.UserProgress, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	if cached, err := h.cache.GetUserProgress(ctx, req.UserId); err == nil {
		return cached, nil
	}

	p, err := h.repo.GetUserProgress(ctx, req)
	if err != nil {
		h.logger.Error("get user progress failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get user progress: %v", err)
	}

	h.cache.SetUserProgress(ctx, p) //nolint:errcheck
	return p, nil
}

// GetProblemStatus returns a user's status for a specific problem.
func (h *ProgressHandler) GetProblemStatus(ctx context.Context, req *progressv1.GetProblemStatusRequest) (*progressv1.ProblemStatus, error) {
	if req.UserId == "" || req.ProblemId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and problem_id are required")
	}

	if cached, err := h.cache.GetProblemStatus(ctx, req.UserId, req.ProblemId); err == nil {
		return cached, nil
	}

	s, err := h.repo.GetProblemStatus(ctx, req)
	if err != nil {
		h.logger.Error("get problem status failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get problem status: %v", err)
	}

	h.cache.SetProblemStatus(ctx, s) //nolint:errcheck
	return s, nil
}

// UpdateProblemStatus updates a user's problem status and aggregate progress.
func (h *ProgressHandler) UpdateProblemStatus(ctx context.Context, req *progressv1.UpdateProblemStatusRequest) (*progressv1.UpdateProblemStatusResponse, error) {
	if req.UserId == "" || req.ProblemId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and problem_id are required")
	}

	resp, err := h.repo.UpdateProblemStatus(ctx, req)
	if err != nil {
		h.logger.Error("update problem status failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "update problem status: %v", err)
	}

	// Invalidate cached progress so next read gets fresh data
	h.cache.InvalidateUserProgress(ctx, req.UserId)

	return resp, nil
}
