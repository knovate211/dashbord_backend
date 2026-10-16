// Package handler implements the gRPC ExecutionService server.
package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/knovate211/execution-service/internal/codegen"
	"github.com/knovate211/execution-service/internal/judge"
	"github.com/knovate211/execution-service/internal/sandbox"
	"github.com/knovate211/execution-service/internal/worker"
	executionv1 "github.com/knovate211/proto/execution/v1"
	problemv1 "github.com/knovate211/proto/problem/v1"
)

// ExecutionHandler implements executionv1.ExecutionServiceServer.
type ExecutionHandler struct {
	executionv1.UnimplementedExecutionServiceServer
	sb      *sandbox.DockerSandbox
	judge   *judge.Judge
	worker  *worker.Worker
	probCli problemv1.ProblemServiceClient
	log     *zap.Logger
}

// New constructs an ExecutionHandler.
func New(sb *sandbox.DockerSandbox, j *judge.Judge, w *worker.Worker, probCli problemv1.ProblemServiceClient, log *zap.Logger) *ExecutionHandler {
	return &ExecutionHandler{sb: sb, judge: j, worker: w, probCli: probCli, log: log}
}

// RunCode executes code synchronously against visible (non-hidden) test cases.
// This maps to the "Run" button on the frontend.
func (h *ExecutionHandler) RunCode(ctx context.Context, req *executionv1.RunCodeRequest) (*executionv1.RunCodeResponse, error) {
	if err := validateRunRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return h.runCases(ctx, req, false)
}

// VerifySolution runs code against every test case, hidden ones included. It
// backs the admin problem editor's "check solution" button: a problem is only
// trustworthy once a known-good solution passes all of its cases, including
// the ones learners never see. The gateway exposes it to admins only.
func (h *ExecutionHandler) VerifySolution(ctx context.Context, req *executionv1.VerifySolutionRequest) (*executionv1.RunCodeResponse, error) {
	run := &executionv1.RunCodeRequest{ProblemId: req.ProblemId, Language: req.Language, Code: req.Code}
	if err := validateRunRequest(run); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return h.runCases(ctx, run, true)
}

// GenerateStarters renders the starter template for every supported language
// from a function-mode signature — the same generator the judge's drivers come
// from, so a starter and the driver wrapped around it always agree.
func (h *ExecutionHandler) GenerateStarters(ctx context.Context, req *executionv1.GenerateStartersRequest) (*executionv1.GenerateStartersResponse, error) {
	sig := codegen.Signature{EntryPoint: strings.TrimSpace(req.EntryPoint), ReturnType: codegen.Type(req.ReturnType)}
	for _, p := range req.Params {
		sig.Params = append(sig.Params, codegen.Param{Name: strings.TrimSpace(p.Name), Type: codegen.Type(p.Type)})
	}
	if err := sig.Validate(); err != nil {
		return &executionv1.GenerateStartersResponse{Error: strings.TrimPrefix(err.Error(), "codegen: ")}, nil
	}
	out := &executionv1.GenerateStartersResponse{Starters: map[string]string{}}
	for _, lang := range []string{codegen.LangJavaScript, codegen.LangPython, codegen.LangJava, codegen.LangCpp, codegen.LangGo} {
		code, err := codegen.Starter(lang, sig)
		if err != nil {
			return &executionv1.GenerateStartersResponse{Error: strings.TrimPrefix(err.Error(), "codegen: ")}, nil
		}
		out.Starters[lang] = code
	}
	return out, nil
}

// runCases executes code against a problem's test cases — visible ones only
// unless includeHidden — and grades each synchronously.
func (h *ExecutionHandler) runCases(ctx context.Context, req *executionv1.RunCodeRequest, includeHidden bool) (*executionv1.RunCodeResponse, error) {
	tcResp, err := h.probCli.GetTestCases(ctx, &problemv1.GetTestCasesRequest{
		ProblemId:     req.ProblemId,
		IncludeHidden: includeHidden,
	})
	if err != nil {
		h.log.Error("fetch test cases for run", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "fetch test cases: %v", err)
	}

	jobId := newJobID()
	var testResults []*executionv1.TestResult
	var maxRuntime int64
	var compileError string

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for _, tc := range tcResp.TestCases {
		sbResult, err := h.sb.Run(runCtx, &sandbox.RunRequest{
			ProblemId:     req.ProblemId,
			Language:      req.Language,
			Code:          req.Code,
			Input:         tc.Input,
			TimeLimitMs:   tc.TimeLimitMs,
			MemoryLimitMb: tc.MemoryLimitMb,
			Spec:          toSandboxSpec(tcResp.ExecutionSpec),
		})
		if err != nil {
			h.log.Error("sandbox run error", zap.Error(err))
			tr := &executionv1.TestResult{
				TestCaseId: tc.Id,
				Status:     judge.StatusRuntimeError,
				Error:      err.Error(),
			}
			testResults = append(testResults, tr)
			continue
		}

		tr := h.judge.EvaluateTestCaseWithSpec(tc, sbResult, tcResp.ExecutionSpec)
		testResults = append(testResults, tr)

		if sbResult.ExecutionMs > maxRuntime {
			maxRuntime = sbResult.ExecutionMs
		}

		// Capture compile error from first failed case
		if tr.Status == judge.StatusCompileError && compileError == "" {
			compileError = tr.Error
		}
	}
	overallStatus := judge.OverallStatus(testResults)

	h.log.Info("run code completed",
		zap.String("job_id", jobId),
		zap.String("language", req.Language),
		zap.String("status", overallStatus),
		zap.Int("cases", len(testResults)),
	)

	return &executionv1.RunCodeResponse{
		JobId:         jobId,
		OverallStatus: overallStatus,
		TestResults:   testResults,
		CompileError:  compileError,
		Runtime:       maxRuntime,
	}, nil
}

// SubmitCode enqueues an async execution job against ALL test cases (including hidden).
// Returns immediately with a job ID. Results are published to NATS execution.result.
func (h *ExecutionHandler) SubmitCode(ctx context.Context, req *executionv1.SubmitCodeRequest) (*executionv1.SubmitCodeResponse, error) {
	if req.SubmissionId == "" || req.ProblemId == "" || req.Language == "" || req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "submission_id, problem_id, language, and code are required")
	}

	if err := h.worker.PublishRunJob(req); err != nil {
		h.log.Error("publish execution job", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "enqueue job: %v", err)
	}

	h.log.Info("submit code enqueued",
		zap.String("submission_id", req.SubmissionId),
		zap.String("language", req.Language),
	)

	return &executionv1.SubmitCodeResponse{JobId: req.SubmissionId}, nil
}

// RunScratchpad executes code verbatim against supplied stdin and returns the
// raw output. There are no test cases and no grading: this backs the in-lesson
// code editor on module assignments, where the learner writes a complete
// program rather than filling in a graded function body.
func (h *ExecutionHandler) RunScratchpad(ctx context.Context, req *executionv1.RunScratchpadRequest) (*executionv1.RunScratchpadResponse, error) {
	if req.Language == "" {
		return nil, status.Error(codes.InvalidArgument, "language is required")
	}
	if strings.TrimSpace(req.Code) == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	supported := map[string]bool{"python": true, "javascript": true, "java": true, "cpp": true, "go": true, "sql": true}
	if !supported[strings.ToLower(req.Language)] {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported language: %s", req.Language)
	}

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	res, err := h.sb.Run(runCtx, &sandbox.RunRequest{
		Language:      strings.ToLower(req.Language),
		Code:          req.Code,
		Input:         req.Stdin,
		TimeLimitMs:   5000,
		MemoryLimitMb: 256,
		Raw:           true,
	})
	if err != nil {
		h.log.Error("scratchpad run failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "run failed: %v", err)
	}

	return &executionv1.RunScratchpadResponse{
		Stdout:      res.Stdout,
		Stderr:      res.Stderr,
		ExitCode:    res.ExitCode,
		ExecutionMs: res.ExecutionMs,
		TimedOut:    res.TimedOut,
	}, nil
}

func validateRunRequest(req *executionv1.RunCodeRequest) error {
	if req.ProblemId == "" {
		return fmt.Errorf("problem_id is required")
	}
	if req.Language == "" {
		return fmt.Errorf("language is required")
	}
	if req.Code == "" {
		return fmt.Errorf("code is required")
	}
	supported := map[string]bool{"python": true, "javascript": true, "java": true, "cpp": true, "go": true, "sql": true}
	if !supported[strings.ToLower(req.Language)] {
		return fmt.Errorf("unsupported language: %s (supported: python, javascript, java, cpp, go, sql)", req.Language)
	}
	return nil
}

func newJobID() string {
	return uuid.New().String()
}
