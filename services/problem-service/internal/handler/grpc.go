// Package handler implements the gRPC ProblemService server.
package handler

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/knovate211/problem-service/internal/cache"
	"github.com/knovate211/problem-service/internal/repository"
	problemv1 "github.com/knovate211/proto/problem/v1"
)

// ProblemHandler implements problemv1.ProblemServiceServer.
type ProblemHandler struct {
	problemv1.UnimplementedProblemServiceServer
	repo   *repository.ProblemRepository
	cache  *cache.ProblemCache
	logger *zap.Logger
}

// New constructs a ProblemHandler with the given dependencies.
func New(repo *repository.ProblemRepository, c *cache.ProblemCache, log *zap.Logger) *ProblemHandler {
	return &ProblemHandler{repo: repo, cache: c, logger: log}
}

// ListProblems returns a filtered, paginated list of problems.
func (h *ProblemHandler) ListProblems(ctx context.Context, req *problemv1.ListProblemsRequest) (*problemv1.ListProblemsResponse, error) {
	// Try cache first
	if cached, err := h.cache.GetListProblems(ctx, req); err == nil {
		return cached, nil
	}

	resp, err := h.repo.ListProblems(ctx, req)
	if err != nil {
		h.logger.Error("list problems failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "list problems: %v", err)
	}

	if err := h.cache.SetListProblems(ctx, req, resp); err != nil {
		h.logger.Warn("cache set list problems failed", zap.Error(err))
	}

	return resp, nil
}

// GetProblem returns the full detail of a single problem by UUID or slug.
func (h *ProblemHandler) GetProblem(ctx context.Context, req *problemv1.GetProblemRequest) (*problemv1.Problem, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Try cache (by id or slug)
	if cached, err := h.cache.GetProblem(ctx, req.Id); err == nil {
		return cached, nil
	}

	p, err := h.repo.GetProblem(ctx, req)
	if errors.Is(err, repository.ErrProblemNotFound) {
		return nil, status.Errorf(codes.NotFound, "problem not found: %s", req.Id)
	}
	if err != nil {
		// A failed lookup is not a missing problem: report it as such, so the
		// caller can tell a deleted problem from a database hiccup.
		h.logger.Error("get problem failed", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "could not load the problem")
	}

	if err := h.cache.SetProblem(ctx, p); err != nil {
		h.logger.Warn("cache set problem failed", zap.Error(err))
	}

	return p, nil
}

// InvalidateProblem drops the cached copies of a problem. The admin screens in
// the API gateway write problems straight to the database, so without this an
// edit stayed invisible, and a deleted problem kept being served, for up to the
// cache TTL.
func (h *ProblemHandler) InvalidateProblem(ctx context.Context, req *problemv1.InvalidateProblemRequest) (*problemv1.InvalidateProblemResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	h.cache.InvalidateProblem(ctx, req.Id, req.Slug)
	return &problemv1.InvalidateProblemResponse{}, nil
}

// GetTestCases returns test cases for a problem.
// This RPC is internal — the API gateway should NOT expose it to the frontend.
func (h *ProblemHandler) GetTestCases(ctx context.Context, req *problemv1.GetTestCasesRequest) (*problemv1.GetTestCasesResponse, error) {
	if req.ProblemId == "" {
		return nil, status.Error(codes.InvalidArgument, "problem_id is required")
	}

	resp, err := h.repo.GetTestCases(ctx, req)
	if err != nil {
		h.logger.Error("get test cases failed", zap.String("problem_id", req.ProblemId), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get test cases: %v", err)
	}

	return resp, nil
}

// ListPracticeSets returns all practice sets with optional per-user progress.
func (h *ProblemHandler) ListPracticeSets(ctx context.Context, req *problemv1.ListPracticeSetsRequest) (*problemv1.ListPracticeSetsResponse, error) {
	resp, err := h.repo.ListPracticeSets(ctx, req)
	if err != nil {
		h.logger.Error("list practice sets failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "list practice sets: %v", err)
	}

	return resp, nil
}

// GetProblemUserStatus returns the submission status for a specific user+problem pair.
func (h *ProblemHandler) GetProblemUserStatus(ctx context.Context, req *problemv1.GetProblemUserStatusRequest) (*problemv1.ProblemUserStatus, error) {
	if req.UserId == "" || req.ProblemId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and problem_id are required")
	}

	// Try cache
	if cached, err := h.cache.GetUserStatus(ctx, req.UserId, req.ProblemId); err == nil {
		return cached, nil
	}

	s, err := h.repo.GetProblemUserStatus(ctx, req)
	if err != nil {
		h.logger.Error("get problem user status failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get problem user status: %v", err)
	}

	if err := h.cache.SetUserStatus(ctx, s); err != nil {
		h.logger.Warn("cache set user status failed", zap.Error(err))
	}

	return s, nil
}
