package resolvers

import (
	"fmt"

	"github.com/graphql-go/graphql"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	executionv1 "github.com/knovate211/proto/execution/v1"
	problemv1 "github.com/knovate211/proto/problem/v1"
	submissionv1 "github.com/knovate211/proto/submission/v1"
)

// SubmissionClients holds all gRPC clients needed for submission resolvers.
type SubmissionClients struct {
	SubmissionSvc submissionv1.SubmissionServiceClient
	ExecutionSvc  executionv1.ExecutionServiceClient
	// ProblemSvc keeps private (test-only) problems out of practice run/submit.
	ProblemSvc problemv1.ProblemServiceClient
	Log        *zap.Logger
}

// GetSubmissionResolver handles the getSubmission GraphQL query.
func (c *SubmissionClients) GetSubmission(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}
	id, ok := p.Args["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("id is required")
	}

	s, err := c.SubmissionSvc.GetSubmission(p.Context, &submissionv1.GetSubmissionRequest{Id: id})
	if err != nil {
		return nil, fmt.Errorf("submission not found: %s", id)
	}
	admin := isAdmin(p.Context)
	if s.UserId != userID && !admin {
		return nil, fmt.Errorf("submission not found: %s", id)
	}

	if !admin {
		redactHidden(s)
	}
	return submissionToMap(s), nil
}

// ListSubmissionsResolver handles the listSubmissions GraphQL query.
func (c *SubmissionClients) ListSubmissions(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	req := &submissionv1.ListSubmissionsRequest{
		UserId:    userID,
		ProblemId: stringArg(p, "problemId"),
		Page:      int32Arg(p, "page", 1),
		PageSize:  int32Arg(p, "pageSize", 20),
	}

	resp, err := c.SubmissionSvc.ListSubmissions(p.Context, req)
	if err != nil {
		return nil, fmt.Errorf("list submissions failed: %v", err)
	}

	submissions := make([]interface{}, 0, len(resp.Submissions))
	for _, s := range resp.Submissions {
		redactHidden(s)
		submissions = append(submissions, submissionToMap(s))
	}

	return map[string]interface{}{
		"submissions": submissions,
		"total":       resp.Total,
		"page":        resp.Page,
		"pageSize":    resp.PageSize,
	}, nil
}

// SubmitCodeResolver handles the submitCode GraphQL mutation.
func (c *SubmissionClients) SubmitCode(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	problemId := stringArg(p, "problemId")
	language := stringArg(p, "language")
	code := stringArg(p, "code")

	if problemId == "" || language == "" || code == "" {
		return nil, fmt.Errorf("problemId, language, and code are required")
	}
	if err := requirePracticeProblem(p.Context, c.ProblemSvc, problemId); err != nil {
		return nil, err
	}

	resp, err := c.SubmissionSvc.Submit(p.Context, &submissionv1.SubmitRequest{
		UserId:    userID,
		ProblemId: problemId,
		Language:  language,
		Code:      code,
	})
	if err != nil {
		c.Log.Error("submit code failed", zap.Error(err))
		return nil, fmt.Errorf("submit failed: %v", err)
	}

	return map[string]interface{}{
		"submissionId": resp.SubmissionId,
	}, nil
}

// RunCodeResolver handles the runCode GraphQL mutation (sync execution against visible test cases).
// RunScratchpad executes code verbatim with supplied stdin and returns the raw
// output. Used by the in-lesson code editor on module assignments, where there
// is no problem and nothing to grade.
func (c *SubmissionClients) RunScratchpad(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	language := stringArg(p, "language")
	code := stringArg(p, "code")
	stdin := stringArg(p, "stdin")

	if language == "" || code == "" {
		return nil, fmt.Errorf("language and code are required")
	}

	resp, err := c.ExecutionSvc.RunScratchpad(p.Context, &executionv1.RunScratchpadRequest{
		Language: language,
		Code:     code,
		Stdin:    stdin,
		UserId:   userID,
	})
	if err != nil {
		c.Log.Error("run scratchpad failed", zap.Error(err))
		return nil, fmt.Errorf("run failed: %v", err)
	}

	return map[string]interface{}{
		"stdout":      resp.Stdout,
		"stderr":      resp.Stderr,
		"exitCode":    int(resp.ExitCode),
		"executionMs": int(resp.ExecutionMs),
		"timedOut":    resp.TimedOut,
	}, nil
}

func (c *SubmissionClients) RunCode(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	problemId := stringArg(p, "problemId")
	language := stringArg(p, "language")
	code := stringArg(p, "code")

	if problemId == "" || language == "" || code == "" {
		return nil, fmt.Errorf("problemId, language, and code are required")
	}
	if err := requirePracticeProblem(p.Context, c.ProblemSvc, problemId); err != nil {
		return nil, err
	}

	resp, err := c.ExecutionSvc.RunCode(p.Context, &executionv1.RunCodeRequest{
		ProblemId: problemId,
		Language:  language,
		Code:      code,
		UserId:    userID,
	})
	if err != nil {
		c.Log.Error("run code failed", zap.Error(err))
		return nil, fmt.Errorf("run code failed: %v", err)
	}

	testResults := make([]interface{}, 0, len(resp.TestResults))
	for _, tr := range resp.TestResults {
		testResults = append(testResults, testResultToMap(tr))
	}

	return map[string]interface{}{
		"jobId":         resp.JobId,
		"overallStatus": resp.OverallStatus,
		"testResults":   testResults,
		"compileError":  resp.CompileError,
		"runtimeMs":     resp.Runtime,
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// redactHidden blanks the content of hidden test cases. The learner still sees
// each hidden case's verdict and time, but never its input, expected output,
// or anything their program printed from it — that would leak the case.
func redactHidden(s *submissionv1.Submission) {
	for _, tr := range s.TestResults {
		if tr.IsHidden {
			tr.Input, tr.ExpectedOutput, tr.ActualOutput, tr.Error = "", "", "", ""
		}
	}
}

func submissionToMap(s *submissionv1.Submission) map[string]interface{} {
	testResults := make([]interface{}, 0, len(s.TestResults))
	for _, tr := range s.TestResults {
		testResults = append(testResults, map[string]interface{}{
			"testCaseId":     tr.TestCaseId,
			"input":          tr.Input,
			"expectedOutput": tr.ExpectedOutput,
			"actualOutput":   tr.ActualOutput,
			"status":         tr.Status,
			"executionMs":    tr.ExecutionMs,
			"error":          tr.Error,
			"isHidden":       tr.IsHidden,
		})
	}
	return map[string]interface{}{
		"id":          s.Id,
		"userId":      s.UserId,
		"problemId":   s.ProblemId,
		"language":    s.Language,
		"status":      s.Status,
		"runtimeMs":   s.RuntimeMs,
		"memoryKb":    s.MemoryKb,
		"testResults": testResults,
		"submittedAt": s.SubmittedAt,
		"completedAt": s.CompletedAt,
	}
}

func testResultToMap(tr *executionv1.TestResult) map[string]interface{} {
	return map[string]interface{}{
		"testCaseId":     tr.TestCaseId,
		"input":          tr.Input,
		"expectedOutput": tr.ExpectedOutput,
		"actualOutput":   tr.ActualOutput,
		"status":         tr.Status,
		"executionMs":    tr.ExecutionMs,
		"error":          tr.Error,
	}
}
