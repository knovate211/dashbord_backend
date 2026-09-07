// Package handler implements the gRPC SubmissionService server.
package handler

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/knovate211/submission-service/internal/orchestrator"
	"github.com/knovate211/submission-service/internal/repository"
	submissionv1 "github.com/knovate211/proto/submission/v1"
)

// SubmissionHandler implements submissionv1.SubmissionServiceServer.
type SubmissionHandler struct {
	submissionv1.UnimplementedSubmissionServiceServer
	orch   *orchestrator.Orchestrator
	repo   *repository.SubmissionRepository
	logger *zap.Logger
}

// New constructs a SubmissionHandler.
func New(orch *orchestrator.Orchestrator, repo *repository.SubmissionRepository, log *zap.Logger) *SubmissionHandler {
	return &SubmissionHandler{orch: orch, repo: repo, logger: log}
}

// Submit creates a new submission and queues it for execution.
func (h *SubmissionHandler) Submit(ctx context.Context, req *submissionv1.SubmitRequest) (*submissionv1.SubmitResponse, error) {
	if req.UserId == "" || req.ProblemId == "" || req.Language == "" || req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id, problem_id, language, and code are required")
	}

	submissionId, err := h.orch.Submit(ctx, req)
	if err != nil {
		h.logger.Error("submit failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "submit: %v", err)
	}

	return &submissionv1.SubmitResponse{SubmissionId: submissionId}, nil
}

// GetSubmission retrieves a single submission by ID.
func (h *SubmissionHandler) GetSubmission(ctx context.Context, req *submissionv1.GetSubmissionRequest) (*submissionv1.Submission, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	s, err := h.repo.GetSubmission(ctx, req.Id)
	if err != nil {
		h.logger.Error("get submission failed", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Errorf(codes.NotFound, "submission not found: %s", req.Id)
	}

	return s, nil
}

// ListSubmissions returns paginated submissions for a user.
func (h *SubmissionHandler) ListSubmissions(ctx context.Context, req *submissionv1.ListSubmissionsRequest) (*submissionv1.ListSubmissionsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	resp, err := h.repo.ListSubmissions(ctx, req)
	if err != nil {
		h.logger.Error("list submissions failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "list submissions: %v", err)
	}

	return resp, nil
}
