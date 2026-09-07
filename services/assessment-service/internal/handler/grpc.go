// Package handler exposes the repository over gRPC.
//
// It is a thin layer: validation that needs the database lives in the
// repository, and this file maps errors onto gRPC status codes and fans out to
// the two services assessments depend on — execution-service for a candidate's
// "Run" against visible test cases, and submission-service for a graded submit.
package handler

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/knovate211/assessment-service/internal/repository"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
	executionv1 "github.com/knovate211/proto/execution/v1"
	submissionv1 "github.com/knovate211/proto/submission/v1"
)

// Handler implements assessmentv1.AssessmentServiceServer.
type Handler struct {
	assessmentv1.UnimplementedAssessmentServiceServer
	repo    *repository.Repo
	submits submissionv1.SubmissionServiceClient
	exec    executionv1.ExecutionServiceClient
	log     *zap.Logger
}

// New constructs a Handler.
func New(repo *repository.Repo, submits submissionv1.SubmissionServiceClient, exec executionv1.ExecutionServiceClient, log *zap.Logger) *Handler {
	return &Handler{repo: repo, submits: submits, exec: exec, log: log}
}

// wrap maps repository errors onto gRPC codes so the gateway can translate them
// into sensible HTTP/GraphQL errors instead of a blanket 500.
func wrap(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, repository.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, repository.ErrAttemptClosed):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, repository.ErrNoAttemptsLeft):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, repository.ErrNotInvited):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

var empty = &assessmentv1.Empty{}

// ─── Companies ────────────────────────────────────────────────────────────────

func (h *Handler) CreateCompany(ctx context.Context, req *assessmentv1.CreateCompanyRequest) (*assessmentv1.Company, error) {
	c, err := h.repo.CreateCompany(ctx, req)
	return c, wrap(err)
}

func (h *Handler) ListCompanies(ctx context.Context, req *assessmentv1.ListCompaniesRequest) (*assessmentv1.ListCompaniesResponse, error) {
	out, err := h.repo.ListCompanies(ctx, req)
	return out, wrap(err)
}

func (h *Handler) AddCompanyMember(ctx context.Context, req *assessmentv1.AddCompanyMemberRequest) (*assessmentv1.Empty, error) {
	return empty, wrap(h.repo.AddCompanyMember(ctx, req))
}

func (h *Handler) ListCompanyMembers(ctx context.Context, req *assessmentv1.ListCompanyMembersRequest) (*assessmentv1.ListCompanyMembersResponse, error) {
	out, err := h.repo.ListCompanyMembers(ctx, req.CompanyId)
	return out, wrap(err)
}

func (h *Handler) Authorize(ctx context.Context, req *assessmentv1.AuthorizeRequest) (*assessmentv1.AuthorizeResponse, error) {
	out, err := h.repo.Authorize(ctx, req)
	return out, wrap(err)
}

// ─── MCQ bank ─────────────────────────────────────────────────────────────────

func (h *Handler) UpsertMcqQuestion(ctx context.Context, req *assessmentv1.UpsertMcqQuestionRequest) (*assessmentv1.UpsertMcqQuestionResponse, error) {
	id, err := h.repo.UpsertMcqQuestion(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	return &assessmentv1.UpsertMcqQuestionResponse{Id: id}, nil
}

func (h *Handler) ListMcqQuestions(ctx context.Context, req *assessmentv1.ListMcqQuestionsRequest) (*assessmentv1.ListMcqQuestionsResponse, error) {
	out, err := h.repo.ListMcqQuestions(ctx, req)
	return out, wrap(err)
}

func (h *Handler) DeleteMcqQuestion(ctx context.Context, req *assessmentv1.DeleteMcqQuestionRequest) (*assessmentv1.Empty, error) {
	return empty, wrap(h.repo.DeleteMcqQuestion(ctx, req.Id))
}

func (h *Handler) BulkImportMcq(ctx context.Context, req *assessmentv1.BulkImportMcqRequest) (*assessmentv1.BulkImportMcqResponse, error) {
	out, err := h.repo.BulkImportMcq(ctx, req)
	return out, wrap(err)
}

// ─── Authoring ────────────────────────────────────────────────────────────────

func (h *Handler) CreateAssessment(ctx context.Context, req *assessmentv1.CreateAssessmentRequest) (*assessmentv1.CreateAssessmentResponse, error) {
	id, err := h.repo.CreateAssessment(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	return &assessmentv1.CreateAssessmentResponse{Id: id}, nil
}

func (h *Handler) UpdateAssessment(ctx context.Context, req *assessmentv1.UpdateAssessmentRequest) (*assessmentv1.Empty, error) {
	return empty, wrap(h.repo.UpdateAssessment(ctx, req))
}

func (h *Handler) GetAssessment(ctx context.Context, req *assessmentv1.GetAssessmentRequest) (*assessmentv1.Assessment, error) {
	a, err := h.repo.GetAssessment(ctx, req.Id, req.IncludeAnswerKey)
	return a, wrap(err)
}

func (h *Handler) ListAssessments(ctx context.Context, req *assessmentv1.ListAssessmentsRequest) (*assessmentv1.ListAssessmentsResponse, error) {
	out, err := h.repo.ListAssessments(ctx, req)
	return out, wrap(err)
}

func (h *Handler) PublishAssessment(ctx context.Context, req *assessmentv1.PublishAssessmentRequest) (*assessmentv1.PublishAssessmentResponse, error) {
	out, err := h.repo.PublishAssessment(ctx, req)
	if err != nil {
		// A failed publish is nearly always the author's mistake (empty
		// section, passing marks above the total), so report it as such.
		if errors.Is(err, repository.ErrNotFound) {
			return nil, wrap(err)
		}
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return out, nil
}

func (h *Handler) DeleteAssessment(ctx context.Context, req *assessmentv1.DeleteAssessmentRequest) (*assessmentv1.Empty, error) {
	return empty, wrap(h.repo.DeleteAssessment(ctx, req.Id))
}

func (h *Handler) UpsertSection(ctx context.Context, req *assessmentv1.UpsertSectionRequest) (*assessmentv1.UpsertSectionResponse, error) {
	id, err := h.repo.UpsertSection(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	return &assessmentv1.UpsertSectionResponse{Id: id}, nil
}

func (h *Handler) DeleteSection(ctx context.Context, req *assessmentv1.DeleteSectionRequest) (*assessmentv1.Empty, error) {
	return empty, wrap(h.repo.DeleteSection(ctx, req.Id))
}

func (h *Handler) SetSectionQuestions(ctx context.Context, req *assessmentv1.SetSectionQuestionsRequest) (*assessmentv1.Empty, error) {
	return empty, wrap(h.repo.SetSectionQuestions(ctx, req))
}

// ─── Invitations ──────────────────────────────────────────────────────────────

func (h *Handler) InviteCandidates(ctx context.Context, req *assessmentv1.InviteCandidatesRequest) (*assessmentv1.InviteCandidatesResponse, error) {
	out, err := h.repo.InviteCandidates(ctx, req)
	return out, wrap(err)
}

func (h *Handler) ListInvites(ctx context.Context, req *assessmentv1.ListInvitesRequest) (*assessmentv1.ListInvitesResponse, error) {
	out, err := h.repo.ListInvites(ctx, req.AssessmentId)
	return out, wrap(err)
}

// ─── Taking a test ────────────────────────────────────────────────────────────

func (h *Handler) ListAvailableAssessments(ctx context.Context, req *assessmentv1.ListAvailableAssessmentsRequest) (*assessmentv1.ListAvailableAssessmentsResponse, error) {
	out, err := h.repo.ListAvailableAssessments(ctx, req)
	return out, wrap(err)
}

func (h *Handler) StartAttempt(ctx context.Context, req *assessmentv1.StartAttemptRequest) (*assessmentv1.AttemptState, error) {
	attemptID, err := h.repo.StartAttempt(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	state, err := h.repo.GetAttemptState(ctx, attemptID, req.UserId)
	return state, wrap(err)
}

func (h *Handler) GetAttemptState(ctx context.Context, req *assessmentv1.GetAttemptStateRequest) (*assessmentv1.AttemptState, error) {
	state, err := h.repo.GetAttemptState(ctx, req.AttemptId, req.UserId)
	return state, wrap(err)
}

func (h *Handler) SaveAnswer(ctx context.Context, req *assessmentv1.SaveAnswerRequest) (*assessmentv1.SaveAnswerResponse, error) {
	left, err := h.repo.SaveAnswer(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	return &assessmentv1.SaveAnswerResponse{Saved: true, SecondsLeft: left}, nil
}

// RunAttemptCode executes against the visible test cases only. Nothing is
// scored and nothing is recorded — this is the candidate's scratch run.
func (h *Handler) RunAttemptCode(ctx context.Context, req *assessmentv1.RunAttemptCodeRequest) (*assessmentv1.RunAttemptCodeResponse, error) {
	problemID, err := h.repo.AttemptCodingQuestion(ctx, req.AttemptId, req.UserId, req.QuestionId)
	if err != nil {
		return nil, wrap(err)
	}

	resp, err := h.exec.RunCode(ctx, &executionv1.RunCodeRequest{
		ProblemId: problemID,
		Language:  req.Language,
		Code:      req.Code,
		UserId:    req.UserId,
	})
	if err != nil {
		h.log.Error("run attempt code failed", zap.Error(err))
		return nil, status.Error(codes.Internal, "could not run your code, please try again")
	}

	out := &assessmentv1.RunAttemptCodeResponse{
		OverallStatus: resp.OverallStatus,
		CompileError:  resp.CompileError,
		RuntimeMs:     resp.Runtime,
		TestResults:   make([]*assessmentv1.AttemptTestResult, 0, len(resp.TestResults)),
	}
	for _, tr := range resp.TestResults {
		out.TestResults = append(out.TestResults, &assessmentv1.AttemptTestResult{
			Input:          tr.Input,
			ExpectedOutput: tr.ExpectedOutput,
			ActualOutput:   tr.ActualOutput,
			Status:         tr.Status,
			ExecutionMs:    tr.ExecutionMs,
			Error:          tr.Error,
		})
	}
	return out, nil
}

// SubmitAttemptCode hands the code to the existing submission pipeline and
// records the link back to this attempt. The verdict arrives asynchronously via
// submission.graded and is applied by the consumer.
func (h *Handler) SubmitAttemptCode(ctx context.Context, req *assessmentv1.SubmitAttemptCodeRequest) (*assessmentv1.SubmitAttemptCodeResponse, error) {
	problemID, err := h.repo.AttemptCodingQuestion(ctx, req.AttemptId, req.UserId, req.QuestionId)
	if err != nil {
		return nil, wrap(err)
	}

	resp, err := h.submits.Submit(ctx, &submissionv1.SubmitRequest{
		ProblemId: problemID,
		Language:  req.Language,
		Code:      req.Code,
		UserId:    req.UserId,
	})
	if err != nil {
		h.log.Error("submit attempt code failed", zap.Error(err))
		return nil, status.Error(codes.Internal, "could not submit your code, please try again")
	}

	left, err := h.repo.RecordCodeSubmission(ctx, req.AttemptId, req.QuestionId, resp.SubmissionId, req.Language, req.Code)
	if err != nil {
		return nil, wrap(err)
	}
	return &assessmentv1.SubmitAttemptCodeResponse{SubmissionId: resp.SubmissionId, SecondsLeft: left}, nil
}

func (h *Handler) GetAttemptSubmission(ctx context.Context, req *assessmentv1.GetAttemptSubmissionRequest) (*assessmentv1.GetAttemptSubmissionResponse, error) {
	out, err := h.repo.GetAttemptSubmissionStatus(ctx, req.AttemptId, req.UserId, req.SubmissionId)
	return out, wrap(err)
}

func (h *Handler) SubmitAttempt(ctx context.Context, req *assessmentv1.SubmitAttemptRequest) (*assessmentv1.AttemptSummary, error) {
	// Ownership is checked before finalizing so one candidate can never end
	// another's test.
	state, err := h.repo.GetAttemptState(ctx, req.AttemptId, req.UserId)
	if err != nil {
		return nil, wrap(err)
	}
	if state.Status != "in_progress" {
		summary, err := h.repo.AttemptSummary(ctx, req.AttemptId)
		return summary, wrap(err)
	}

	reason := req.Reason
	if reason == "" {
		reason = "user"
	}
	summary, err := h.repo.FinalizeAttempt(ctx, req.AttemptId, reason)
	return summary, wrap(err)
}

func (h *Handler) RecordProctorEvent(ctx context.Context, req *assessmentv1.RecordProctorEventRequest) (*assessmentv1.RecordProctorEventResponse, error) {
	out, err := h.repo.RecordProctorEvent(ctx, req)
	return out, wrap(err)
}

func (h *Handler) ListMyAttempts(ctx context.Context, req *assessmentv1.ListMyAttemptsRequest) (*assessmentv1.ListMyAttemptsResponse, error) {
	attempts, err := h.repo.ListMyAttempts(ctx, req.UserId)
	if err != nil {
		return nil, wrap(err)
	}
	return &assessmentv1.ListMyAttemptsResponse{Attempts: attempts}, nil
}

func (h *Handler) GetAttemptResult(ctx context.Context, req *assessmentv1.GetAttemptResultRequest) (*assessmentv1.AttemptResult, error) {
	out, err := h.repo.GetAttemptResult(ctx, req.AttemptId, req.UserId)
	return out, wrap(err)
}

// ─── Reporting & shortlisting ─────────────────────────────────────────────────

func (h *Handler) ListAttempts(ctx context.Context, req *assessmentv1.ListAttemptsRequest) (*assessmentv1.ListAttemptsResponse, error) {
	out, err := h.repo.ListAttempts(ctx, req)
	return out, wrap(err)
}

func (h *Handler) GetAttemptReport(ctx context.Context, req *assessmentv1.GetAttemptReportRequest) (*assessmentv1.AttemptReport, error) {
	out, err := h.repo.GetAttemptReport(ctx, req.AttemptId)
	return out, wrap(err)
}

func (h *Handler) GradeDescriptive(ctx context.Context, req *assessmentv1.GradeDescriptiveRequest) (*assessmentv1.GradeDescriptiveResponse, error) {
	out, err := h.repo.GradeDescriptive(ctx, req)
	return out, wrap(err)
}

func (h *Handler) ComputeShortlist(ctx context.Context, req *assessmentv1.ComputeShortlistRequest) (*assessmentv1.Shortlist, error) {
	out, err := h.repo.ComputeShortlist(ctx, req)
	return out, wrap(err)
}

func (h *Handler) ListShortlists(ctx context.Context, req *assessmentv1.ListShortlistsRequest) (*assessmentv1.ListShortlistsResponse, error) {
	out, err := h.repo.ListShortlists(ctx, req.AssessmentId)
	return out, wrap(err)
}

func (h *Handler) GetShortlist(ctx context.Context, req *assessmentv1.GetShortlistRequest) (*assessmentv1.Shortlist, error) {
	out, err := h.repo.GetShortlist(ctx, req.Id)
	return out, wrap(err)
}

func (h *Handler) SetCandidateDecision(ctx context.Context, req *assessmentv1.SetCandidateDecisionRequest) (*assessmentv1.Empty, error) {
	return empty, wrap(h.repo.SetCandidateDecision(ctx, req))
}

func (h *Handler) ExportResults(ctx context.Context, req *assessmentv1.ExportResultsRequest) (*assessmentv1.ExportResultsResponse, error) {
	out, err := h.repo.ExportResults(ctx, req)
	return out, wrap(err)
}
