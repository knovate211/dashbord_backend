package resolvers

import (
	"fmt"

	"github.com/graphql-go/graphql"
	"go.uber.org/zap"
	"google.golang.org/grpc/status"

	"github.com/knovate211/api-gateway/middleware"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// AssessmentClients holds the gRPC client for the student-facing test player.
//
// Everything here is scoped to the caller's own attempts: the user id always
// comes from the validated JWT, never from an argument, so no candidate can
// read or write another's paper.
type AssessmentClients struct {
	AssessmentSvc assessmentv1.AssessmentServiceClient
	Log           *zap.Logger
}

func requireUser(p graphql.ResolveParams) (string, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return "", fmt.Errorf("authentication required")
	}
	return userID, nil
}

// ─── Queries ──────────────────────────────────────────────────────────────────

// ListAssessments returns the tests visible to the signed-in student.
func (c *AssessmentClients) ListAssessments(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	resp, err := c.AssessmentSvc.ListAvailableAssessments(p.Context, &assessmentv1.ListAvailableAssessmentsRequest{
		UserId: userID,
		Scope:  stringArg(p, "scope"),
	})
	if err != nil {
		c.Log.Error("list assessments failed", zap.Error(err))
		return nil, fmt.Errorf("could not load tests")
	}

	out := make([]interface{}, 0, len(resp.Assessments))
	for _, a := range resp.Assessments {
		out = append(out, map[string]interface{}{
			"id":              a.Id,
			"title":           a.Title,
			"description":     a.Description,
			"purpose":         a.Purpose,
			"companyName":     a.CompanyName,
			"companyLogo":     a.CompanyLogo,
			"durationMinutes": int(a.DurationMinutes),
			"totalMarks":      int(a.TotalMarks),
			"questionCount":   int(a.QuestionCount),
			"sectionSummary":  a.SectionSummary,
			"opensAt":         a.OpensAt,
			"closesAt":        a.ClosesAt,
			"maxAttempts":     int(a.MaxAttempts),
			"attemptsUsed":    int(a.AttemptsUsed),
			"liveAttemptId":   a.LiveAttemptId,
			"canStart":        a.CanStart,
			"blockedReason":   a.BlockedReason,
			// Nil for a paper with no proctoring configured, which the client
			// reads as "nothing is monitored".
			"proctoring": func() interface{} {
				if a.Proctoring == nil {
					return nil
				}
				return map[string]interface{}{
					"requireFullscreen": a.Proctoring.RequireFullscreen,
					"tabSwitchLimit":    int(a.Proctoring.TabSwitchLimit),
					"blockCopyPaste":    a.Proctoring.BlockCopyPaste,
					"webcam":            a.Proctoring.Webcam,
				}
			}(),
		})
	}
	return out, nil
}

// GetAttemptState is the player's resume endpoint: the full paper, the saved
// answers and the authoritative time remaining.
func (c *AssessmentClients) GetAttemptState(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	state, err := c.AssessmentSvc.GetAttemptState(p.Context, &assessmentv1.GetAttemptStateRequest{
		AttemptId: stringArg(p, "attemptId"),
		UserId:    userID,
	})
	if err != nil {
		return nil, cleanErr(err, "could not load your test")
	}
	return attemptStateToMap(state), nil
}

// GetMyAttempts returns the student's own attempt history.
func (c *AssessmentClients) GetMyAttempts(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	resp, err := c.AssessmentSvc.ListMyAttempts(p.Context, &assessmentv1.ListMyAttemptsRequest{UserId: userID})
	if err != nil {
		c.Log.Error("list my attempts failed", zap.Error(err))
		return nil, fmt.Errorf("could not load your attempts")
	}

	out := make([]interface{}, 0, len(resp.Attempts))
	for _, a := range resp.Attempts {
		out = append(out, attemptSummaryToMap(a))
	}
	return out, nil
}

// GetAttemptResult returns the student's result. The per-question breakdown is
// present only when the test allows revealing it.
func (c *AssessmentClients) GetAttemptResult(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	res, err := c.AssessmentSvc.GetAttemptResult(p.Context, &assessmentv1.GetAttemptResultRequest{
		AttemptId: stringArg(p, "attemptId"),
		UserId:    userID,
	})
	if err != nil {
		return nil, cleanErr(err, "could not load your result")
	}

	questions := make([]interface{}, 0, len(res.Questions))
	for _, q := range res.Questions {
		questions = append(questions, attemptQuestionToMap(q))
	}
	return map[string]interface{}{
		"summary":   attemptSummaryToMap(res.Summary),
		"questions": questions,
		"revealed":  res.Revealed,
		"withheld":  res.Withheld,
	}, nil
}

// ─── Mutations ────────────────────────────────────────────────────────────────

// StartAttempt begins (or resumes) a test.
func (c *AssessmentClients) StartAttempt(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	state, err := c.AssessmentSvc.StartAttempt(p.Context, &assessmentv1.StartAttemptRequest{
		AssessmentId: stringArg(p, "assessmentId"),
		UserId:       userID,
		InviteToken:  stringArg(p, "inviteToken"),
	})
	if err != nil {
		return nil, cleanErr(err, "could not start this test")
	}
	return attemptStateToMap(state), nil
}

// SaveAnswer autosaves an MCQ selection or a written answer.
func (c *AssessmentClients) SaveAnswer(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	resp, err := c.AssessmentSvc.SaveAnswer(p.Context, &assessmentv1.SaveAnswerRequest{
		AttemptId:         stringArg(p, "attemptId"),
		UserId:            userID,
		QuestionId:        stringArg(p, "questionId"),
		SelectedOptionIds: stringListArg(p, "selectedOptionIds"),
		TextAnswer:        stringArg(p, "textAnswer"),
		TimeSpentMs:       int64(int32Arg(p, "timeSpentMs", 0)),
		MarkedReview:      boolArg(p, "markedReview"),
		ClearAnswer:       boolArg(p, "clearAnswer"),
		Language:          stringArg(p, "language"),
		Code:              stringArg(p, "code"),
	})
	if err != nil {
		return nil, cleanErr(err, "could not save your answer")
	}
	return map[string]interface{}{"saved": resp.Saved, "secondsLeft": int(resp.SecondsLeft)}, nil
}

// RunAttemptCode runs the candidate's code against the visible test cases only.
func (c *AssessmentClients) RunAttemptCode(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	resp, err := c.AssessmentSvc.RunAttemptCode(p.Context, &assessmentv1.RunAttemptCodeRequest{
		AttemptId:  stringArg(p, "attemptId"),
		UserId:     userID,
		QuestionId: stringArg(p, "questionId"),
		Language:   stringArg(p, "language"),
		Code:       stringArg(p, "code"),
	})
	if err != nil {
		return nil, cleanErr(err, "could not run your code")
	}

	results := make([]interface{}, 0, len(resp.TestResults))
	for _, tr := range resp.TestResults {
		results = append(results, map[string]interface{}{
			"input":          tr.Input,
			"expectedOutput": tr.ExpectedOutput,
			"actualOutput":   tr.ActualOutput,
			"status":         tr.Status,
			"executionMs":    int(tr.ExecutionMs),
			"error":          tr.Error,
		})
	}
	return map[string]interface{}{
		"overallStatus": resp.OverallStatus,
		"testResults":   results,
		"compileError":  resp.CompileError,
		"runtimeMs":     int(resp.RuntimeMs),
	}, nil
}

// SubmitAttemptCode queues a graded submission for a coding question.
func (c *AssessmentClients) SubmitAttemptCode(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	resp, err := c.AssessmentSvc.SubmitAttemptCode(p.Context, &assessmentv1.SubmitAttemptCodeRequest{
		AttemptId:  stringArg(p, "attemptId"),
		UserId:     userID,
		QuestionId: stringArg(p, "questionId"),
		Language:   stringArg(p, "language"),
		Code:       stringArg(p, "code"),
	})
	if err != nil {
		return nil, cleanErr(err, "could not submit your code")
	}
	return map[string]interface{}{
		"submissionId": resp.SubmissionId,
		"secondsLeft":  int(resp.SecondsLeft),
	}, nil
}

// GetAttemptSubmission polls the judge verdict for a queued submission.
func (c *AssessmentClients) GetAttemptSubmission(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	resp, err := c.AssessmentSvc.GetAttemptSubmission(p.Context, &assessmentv1.GetAttemptSubmissionRequest{
		AttemptId:    stringArg(p, "attemptId"),
		UserId:       userID,
		SubmissionId: stringArg(p, "submissionId"),
	})
	if err != nil {
		return nil, cleanErr(err, "could not load the submission result")
	}
	return map[string]interface{}{
		"submissionId": resp.SubmissionId,
		"status":       resp.Status,
		"passedCount":  int(resp.PassedCount),
		"totalCount":   int(resp.TotalCount),
		"compileError": resp.CompileError,
		"runtimeMs":    int(resp.RuntimeMs),
	}, nil
}

// SubmitAttempt ends the test and grades it.
func (c *AssessmentClients) SubmitAttempt(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	summary, err := c.AssessmentSvc.SubmitAttempt(p.Context, &assessmentv1.SubmitAttemptRequest{
		AttemptId: stringArg(p, "attemptId"),
		UserId:    userID,
		Reason:    "user",
	})
	if err != nil {
		return nil, cleanErr(err, "could not submit your test")
	}
	return attemptSummaryToMap(summary), nil
}

// RecordProctorEvent appends a client-observed integrity signal.
func (c *AssessmentClients) RecordProctorEvent(p graphql.ResolveParams) (interface{}, error) {
	userID, err := requireUser(p)
	if err != nil {
		return nil, err
	}

	resp, err := c.AssessmentSvc.RecordProctorEvent(p.Context, &assessmentv1.RecordProctorEventRequest{
		AttemptId: stringArg(p, "attemptId"),
		UserId:    userID,
		Kind:      stringArg(p, "kind"),
		Detail:    stringArg(p, "detail"),
	})
	if err != nil {
		// A dropped proctoring signal must never break the candidate's test —
		// report it as a no-op rather than an error the player has to handle.
		c.Log.Warn("record proctor event failed", zap.Error(err))
		return map[string]interface{}{"integrityScore": 100.0, "terminated": false, "warning": ""}, nil
	}
	return map[string]interface{}{
		"integrityScore": resp.IntegrityScore,
		"terminated":     resp.Terminated,
		"warning":        resp.Warning,
	}, nil
}

// ─── Mapping helpers ──────────────────────────────────────────────────────────

func attemptStateToMap(s *assessmentv1.AttemptState) map[string]interface{} {
	sections := make([]interface{}, 0, len(s.Sections))
	for _, sec := range s.Sections {
		sections = append(sections, map[string]interface{}{
			"id":              sec.Id,
			"title":           sec.Title,
			"kind":            sec.Kind,
			"orderIndex":      int(sec.OrderIndex),
			"durationMinutes": int(sec.DurationMinutes),
		})
	}
	questions := make([]interface{}, 0, len(s.Questions))
	for _, q := range s.Questions {
		questions = append(questions, attemptQuestionToMap(q))
	}

	proctoring := map[string]interface{}{
		"requireFullscreen": false, "tabSwitchLimit": 0, "blockCopyPaste": false, "webcam": false,
	}
	if s.Proctoring != nil {
		proctoring = map[string]interface{}{
			"requireFullscreen": s.Proctoring.RequireFullscreen,
			"tabSwitchLimit":    int(s.Proctoring.TabSwitchLimit),
			"blockCopyPaste":    s.Proctoring.BlockCopyPaste,
			"webcam":            s.Proctoring.Webcam,
		}
	}

	return map[string]interface{}{
		"attemptId":       s.AttemptId,
		"assessmentId":    s.AssessmentId,
		"title":           s.Title,
		"status":          s.Status,
		"allowBacktrack":  s.AllowBacktrack,
		"lockForward":     s.LockForward,
		"proctoring":      proctoring,
		"serverNow":       s.ServerNow,
		"expiresAt":       s.ExpiresAt,
		"secondsLeft":     int(s.SecondsLeft),
		"sections":        sections,
		"questions":       questions,
		"maxScore":        s.MaxScore,
		"negativeMarking": s.NegativeMarking,
		"resultsWithheld": s.ResultsWithheld,
	}
}

func attemptQuestionToMap(q *assessmentv1.AttemptQuestion) map[string]interface{} {
	options := make([]interface{}, 0, len(q.Options))
	for _, o := range q.Options {
		options = append(options, map[string]interface{}{
			"id":         o.Id,
			"body":       o.Body,
			"isCorrect":  o.IsCorrect,
			"orderIndex": int(o.OrderIndex),
		})
	}
	selected := make([]interface{}, 0, len(q.SelectedOptionIds))
	for _, id := range q.SelectedOptionIds {
		selected = append(selected, id)
	}

	var awarded interface{}
	if q.AwardedMarks != nil {
		awarded = *q.AwardedMarks
	}

	return map[string]interface{}{
		"id":                q.Id,
		"sectionId":         q.SectionId,
		"kind":              q.Kind,
		"orderIndex":        int(q.OrderIndex),
		"marks":             q.Marks,
		"body":              q.Body,
		"mcqKind":           q.McqKind,
		"options":           options,
		"problemId":         q.ProblemId,
		"problemTitle":      q.ProblemTitle,
		"selectedOptionIds": selected,
		"textAnswer":        q.TextAnswer,
		"submissionId":      q.SubmissionId,
		"language":          q.Language,
		"code":              q.Code,
		"gradingStatus":     q.GradingStatus,
		"visited":           q.Visited,
		"markedReview":      q.MarkedReview,
		"timeSpentMs":       int(q.TimeSpentMs),
		"awardedMarks":      awarded,
	}
}

func attemptSummaryToMap(s *assessmentv1.AttemptSummary) map[string]interface{} {
	if s == nil {
		return nil
	}
	return map[string]interface{}{
		"id":             s.Id,
		"assessmentId":   s.AssessmentId,
		"assessmentName": s.AssessmentName,
		"attemptNo":      int(s.AttemptNo),
		"status":         s.Status,
		"startedAt":      s.StartedAt,
		"submittedAt":    s.SubmittedAt,
		"evaluatedAt":    s.EvaluatedAt,
		"score":          s.Score,
		"maxScore":       s.MaxScore,
		"percent":        s.Percent,
		"integrityScore": s.IntegrityScore,
		"passed":         s.Passed,
	}
}

// cleanErr surfaces the service's own message (which is written for the
// candidate: "your test has ended", "you are not invited") rather than a raw
// gRPC status string.
func cleanErr(err error, fallback string) error {
	if st, ok := status.FromError(err); ok && st.Message() != "" {
		return fmt.Errorf("%s", st.Message())
	}
	return fmt.Errorf("%s", fallback)
}

func stringListArg(p graphql.ResolveParams, key string) []string {
	raw, ok := p.Args[key].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func boolArg(p graphql.ResolveParams, key string) bool {
	v, _ := p.Args[key].(bool)
	return v
}
