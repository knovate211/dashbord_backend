package resolvers

import (
	"context"
	"fmt"

	"github.com/knovate211/api-gateway/middleware"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
	problemv1 "github.com/knovate211/proto/problem/v1"
)

// Private problems are the coding questions of hiring and scholarship tests.
// Their statement is served only inside the candidate's own attempt, and they
// can never be run or submitted through practice — that would let a candidate
// grade answers outside the clock, or read them before the test opens.

func isAdmin(ctx context.Context) bool {
	return middleware.RoleFromContext(ctx) == "admin"
}

// attemptHasProblem reports whether attemptID belongs to userID and contains
// problemID. GetAttemptState enforces the ownership check itself.
func attemptHasProblem(ctx context.Context, svc assessmentv1.AssessmentServiceClient, userID, attemptID, problemID string) bool {
	if svc == nil || userID == "" || attemptID == "" {
		return false
	}
	state, err := svc.GetAttemptState(ctx, &assessmentv1.GetAttemptStateRequest{AttemptId: attemptID, UserId: userID})
	if err != nil {
		return false
	}
	for _, q := range state.Questions {
		if q.ProblemId == problemID {
			return true
		}
	}
	return false
}

// requirePracticeProblem rejects a practice run or submit against a private
// problem. Tests reach the judge through runAttemptCode/submitAttemptCode.
func requirePracticeProblem(ctx context.Context, svc problemv1.ProblemServiceClient, problemID string) error {
	if svc == nil || isAdmin(ctx) {
		return nil
	}
	prob, err := svc.GetProblem(ctx, &problemv1.GetProblemRequest{Id: problemID})
	if err != nil {
		return fmt.Errorf("problem not found: %s", problemID)
	}
	if prob.IsPrivate {
		return fmt.Errorf("problem not found: %s", problemID)
	}
	return nil
}
