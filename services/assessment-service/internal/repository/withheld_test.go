package repository

import (
	"testing"

	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// TestResultsWithheld pins down which papers hide their score from the person
// who sat them.
//
// The direction matters. Unlike inviteOnly — a fail-closed guard, where an
// unrecognised purpose must be treated as restricted — this is an allowlist,
// and that is deliberate: a purpose nobody has considered yet should keep
// the ordinary behaviour of showing a candidate their result, not silently
// start hiding it.
func TestResultsWithheld(t *testing.T) {
	tests := []struct {
		purpose string
		want    bool
		why     string
	}{
		{"scholarship", true, "a scholarship result is a fee decision staff review and email out"},
		{"practice", false, "practice is worthless if you cannot see how you did"},
		{"hiring", true, "a hiring result goes to the company, who get back to the candidate"},
		{"  Hiring ", true, "case and padding must not be a way past the redaction either"},
		{"", false, "an unknown purpose keeps the existing behaviour rather than hiding results"},
		{"Scholarship", true, "case must not be a way past the redaction"},
		{"  scholarship  ", true, "nor must padding"},
	}

	for _, tc := range tests {
		t.Run(tc.purpose, func(t *testing.T) {
			if got := resultsWithheld(tc.purpose); got != tc.want {
				t.Errorf("resultsWithheld(%q) = %v, want %v — %s", tc.purpose, got, tc.want, tc.why)
			}
		})
	}
}

// TestRedactWithheldLeavesNoMarks is the assertion that actually protects the
// candidate: not that a flag is set, but that every field carrying a mark is
// cleared. A new scoring field added to AttemptSummary and forgotten here is
// exactly how this leaks back, so the check is per-field and explicit.
func TestRedactWithheldLeavesNoMarks(t *testing.T) {
	s := &assessmentv1.AttemptSummary{
		AssessmentName: "Knovate Scholarship Test",
		Status:         "evaluated",
		SubmittedAt:    "2026-08-20T16:00:00Z",
		Purpose:        "scholarship",
		Score:          87,
		MaxScore:       100,
		Percent:        87,
		Passed:         true,
		Decision:       "shortlisted",
		SectionScores:  map[string]float64{"mcq": 40},
		SectionMax:     map[string]float64{"mcq": 50},
	}
	redactWithheld(s)

	if s.Score != 0 || s.MaxScore != 0 || s.Percent != 0 {
		t.Errorf("marks survived redaction: score=%v max=%v percent=%v", s.Score, s.MaxScore, s.Percent)
	}
	if s.Passed {
		t.Error("passed survived redaction — a verdict is a result too")
	}
	if s.Decision != "" {
		t.Errorf("decision survived redaction: %q", s.Decision)
	}
	if s.SectionScores != nil || s.SectionMax != nil {
		t.Error("per-section marks survived redaction — the total can be reassembled from them")
	}

	// What the candidate is still owed: confirmation their paper arrived.
	if s.AssessmentName == "" || s.Status == "" || s.SubmittedAt == "" {
		t.Error("redaction removed the confirmation as well; silence reads as a system failure")
	}
}

// TestRedactWithheldSparesOtherPapers guards the blast radius. Practice
// summaries pass through this same helper and must come out untouched.
// (Recruiter reports never call it, so hiring marks stay visible to them.)
func TestRedactWithheldSparesOtherPapers(t *testing.T) {
	for _, purpose := range []string{"practice", ""} {
		t.Run(purpose, func(t *testing.T) {
			s := &assessmentv1.AttemptSummary{Purpose: purpose, Score: 87, MaxScore: 100, Percent: 87, Passed: true}
			redactWithheld(s)
			if s.Score != 87 || s.MaxScore != 100 || s.Percent != 87 || !s.Passed {
				t.Errorf("redaction touched a %q paper: %+v", purpose, s)
			}
		})
	}
}

// A nil summary reaches this helper whenever an upstream query found nothing.
func TestRedactWithheldNilIsSafe(t *testing.T) {
	redactWithheld(nil)
}
