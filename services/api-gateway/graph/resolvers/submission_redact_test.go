package resolvers

import (
	"testing"

	submissionv1 "github.com/knovate211/proto/submission/v1"
)

func TestRedactHiddenBlanksOnlyHiddenCases(t *testing.T) {
	s := &submissionv1.Submission{TestResults: []*submissionv1.TestResult{
		{Input: "1 2", ExpectedOutput: "3", ActualOutput: "3", Status: "Accepted"},
		{Input: "secret", ExpectedOutput: "42", ActualOutput: "41", Status: "WrongAnswer", Error: "printed secret", IsHidden: true, ExecutionMs: 7},
	}}

	redactHidden(s)

	visible, hidden := s.TestResults[0], s.TestResults[1]
	if visible.Input != "1 2" || visible.ExpectedOutput != "3" || visible.ActualOutput != "3" {
		t.Fatalf("visible case was altered: %+v", visible)
	}
	if hidden.Input != "" || hidden.ExpectedOutput != "" || hidden.ActualOutput != "" || hidden.Error != "" {
		t.Fatalf("hidden case leaked content: %+v", hidden)
	}
	if hidden.Status != "WrongAnswer" || hidden.ExecutionMs != 7 {
		t.Fatalf("hidden case lost its verdict: %+v", hidden)
	}
}
