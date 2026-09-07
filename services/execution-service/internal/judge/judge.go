// Package judge compares sandbox output against expected output and produces verdicts.
package judge

import (
	"strings"

	"github.com/knovate211/execution-service/internal/sandbox"
	executionv1 "github.com/knovate211/proto/execution/v1"
	problemv1 "github.com/knovate211/proto/problem/v1"
)

// Status constants for test results.
const (
	StatusAccepted            = "Accepted"
	StatusWrongAnswer         = "WrongAnswer"
	StatusTimeLimitExceeded   = "TimeLimitExceeded"
	StatusMemoryLimitExceeded = "MemoryLimitExceeded"
	StatusRuntimeError        = "RuntimeError"
	StatusCompileError        = "CompileError"
)

// Judge evaluates sandbox results against expected outputs.
type Judge struct{}

// New creates a Judge.
func New() *Judge { return &Judge{} }

// EvaluateTestCase compares sandbox output to a test case and returns a
// TestResult, using the legacy text comparison.
//
// Deprecated: prefer EvaluateTestCaseWithSpec, which honours the problem's
// declared comparison mode. Retained so callers that have no spec keep working.
func (j *Judge) EvaluateTestCase(tc *problemv1.TestCase, res *sandbox.RunResult) *executionv1.TestResult {
	return j.EvaluateTestCaseWithSpec(tc, res, nil)
}

// EvaluateTestCaseWithSpec grades one test case against the problem's declared
// execution contract.
//
// When the problem is function-mode, output is compared structurally: both
// sides are decoded as JSON and matched by value, with optional order- and
// float-insensitivity. Text comparison made the verdict depend on how a
// language happened to format its output — Python's "[12, 8, 20, 5]" lost to
// "[12,8,20,5]" and a string result lost on its quotes — none of which are
// wrong answers.
func (j *Judge) EvaluateTestCaseWithSpec(tc *problemv1.TestCase, res *sandbox.RunResult, spec *problemv1.ExecutionSpec) *executionv1.TestResult {
	tr := &executionv1.TestResult{
		TestCaseId:     tc.Id,
		Input:          tc.Input,
		ExpectedOutput: tc.ExpectedOutput,
		ActualOutput:   strings.TrimSpace(res.Stdout),
		ExecutionMs:    res.ExecutionMs,
	}

	switch {
	case res.TimedOut:
		tr.Status = StatusTimeLimitExceeded
		tr.Error = "Time limit exceeded"

	case res.OOMKilled:
		tr.Status = StatusMemoryLimitExceeded
		tr.Error = "Memory limit exceeded"

	case res.ExitCode != 0:
		tr.Status = StatusRuntimeError
		tr.Error = strings.TrimSpace(res.Stderr)
		if tr.Error == "" {
			tr.Error = "Non-zero exit code"
		}

	case j.matches(tc.ExpectedOutput, res.Stdout, spec):
		tr.Status = StatusAccepted

	default:
		tr.Status = StatusWrongAnswer
	}

	return tr
}

// matches picks the comparison the problem declares. Only function-mode
// problems get structural comparison: stdio problems print free-form text and
// SQL problems come back as result sets, both of which have their own rules.
func (j *Judge) matches(expected, actual string, spec *problemv1.ExecutionSpec) bool {
	if spec == nil || !strings.EqualFold(spec.IoMode, "function") {
		return matchesExpected(expected, actual)
	}
	return MatchesTyped(expected, actual, CompareSpec{
		Mode: CompareMode(spec.Compare),
		Eps:  spec.FloatEps,
	})
}

// OverallStatus computes the aggregate status from individual test results.
// Priority: CompileError > TLE > MLE > RuntimeError > WrongAnswer > Accepted
func OverallStatus(results []*executionv1.TestResult) string {
	if len(results) == 0 {
		return StatusAccepted
	}

	priority := map[string]int{
		StatusCompileError:        10,
		StatusTimeLimitExceeded:   9,
		StatusMemoryLimitExceeded: 8,
		StatusRuntimeError:        7,
		StatusWrongAnswer:         6,
		StatusAccepted:            0,
	}

	worst := StatusAccepted
	for _, r := range results {
		if priority[r.Status] > priority[worst] {
			worst = r.Status
		}
	}
	return worst
}

// matchesExpected decides whether the sandbox output satisfies the test case.
//
// SQL submissions return a JSON result set, which cannot be graded by string
// comparison: column names come back lower-cased by PostgreSQL, numbers are
// formatted differently, and most problems permit any row order. When BOTH
// sides are result sets they are compared semantically; everything else keeps
// the original whitespace-normalised text comparison.
func matchesExpected(expected, actual string) bool {
	expectedRS, expectedOK := parseResultSet(expected)
	actualRS, actualOK := parseResultSet(actual)
	if expectedOK && actualOK {
		return resultSetsEqual(expectedRS, actualRS)
	}
	return normalize(actual) == normalize(expected)
}

// normalize trims trailing whitespace and normalizes line endings for comparison.
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		trimmed := strings.TrimRight(l, " \t\r")
		out = append(out, trimmed)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}
