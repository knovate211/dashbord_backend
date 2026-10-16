package judge

import (
	"testing"

	executionv1 "github.com/knovate211/proto/execution/v1"
	problemv1 "github.com/knovate211/proto/problem/v1"
	"github.com/knovate211/execution-service/internal/sandbox"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello\r\nworld", "hello\nworld"},
		{"hello \nworld\t", "hello\nworld"},
		{"hello\n\n", "hello"},
		{"  hello  \n  world  ", "  hello\n  world"},
	}

	for _, tc := range tests {
		got := normalize(tc.input)
		if got != tc.expected {
			t.Errorf("normalize(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestOverallStatus(t *testing.T) {
	tests := []struct {
		results  []*executionv1.TestResult
		expected string
	}{
		{
			results: []*executionv1.TestResult{
				{Status: StatusAccepted},
				{Status: StatusAccepted},
			},
			expected: StatusAccepted,
		},
		{
			results: []*executionv1.TestResult{
				{Status: StatusAccepted},
				{Status: StatusWrongAnswer},
				{Status: StatusAccepted},
			},
			expected: StatusWrongAnswer,
		},
		{
			results: []*executionv1.TestResult{
				{Status: StatusWrongAnswer},
				{Status: StatusRuntimeError},
			},
			expected: StatusRuntimeError,
		},
		{
			results: []*executionv1.TestResult{
				{Status: StatusRuntimeError},
				{Status: StatusTimeLimitExceeded},
			},
			expected: StatusTimeLimitExceeded,
		},
		{
			results: []*executionv1.TestResult{
				{Status: StatusTimeLimitExceeded},
				{Status: StatusCompileError},
			},
			expected: StatusCompileError,
		},
	}

	for _, tc := range tests {
		got := OverallStatus(tc.results)
		if got != tc.expected {
			t.Errorf("OverallStatus = %q; want %q", got, tc.expected)
		}
	}
}

func TestEvaluateTestCase(t *testing.T) {
	j := New()

	tc := &problemv1.TestCase{
		Id:             "tc1",
		Input:          "5",
		ExpectedOutput: "10",
	}

	// 1. Accepted
	res1 := &sandbox.RunResult{Stdout: "10\n", ExitCode: 0}
	tr1 := j.EvaluateTestCase(tc, res1)
	if tr1.Status != StatusAccepted {
		t.Errorf("expected StatusAccepted, got %s", tr1.Status)
	}

	// 2. Wrong Answer
	res2 := &sandbox.RunResult{Stdout: "11\n", ExitCode: 0}
	tr2 := j.EvaluateTestCase(tc, res2)
	if tr2.Status != StatusWrongAnswer {
		t.Errorf("expected StatusWrongAnswer, got %s", tr2.Status)
	}

	// 3. Timed Out
	res3 := &sandbox.RunResult{TimedOut: true}
	tr3 := j.EvaluateTestCase(tc, res3)
	if tr3.Status != StatusTimeLimitExceeded {
		t.Errorf("expected StatusTimeLimitExceeded, got %s", tr3.Status)
	}

	// 4. RuntimeError
	res4 := &sandbox.RunResult{ExitCode: 1, Stderr: "IndexError"}
	tr4 := j.EvaluateTestCase(tc, res4)
	if tr4.Status != StatusRuntimeError || tr4.Error != "IndexError" {
		t.Errorf("expected StatusRuntimeError with error 'IndexError', got status %s error %s", tr4.Status, tr4.Error)
	}
}
