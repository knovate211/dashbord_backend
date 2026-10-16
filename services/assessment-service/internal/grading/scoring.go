// Package grading holds the pure scoring rules for an attempt. Everything here
// is deliberately free of database and network access so the marking policy can
// be unit-tested in isolation — it is the part of the system a candidate is
// most likely to dispute.
package grading

import (
	"math"
	"strconv"
	"strings"
)

// McqInput describes one answered multiple-choice question.
type McqInput struct {
	Kind            string // single | multiple | numeric
	Marks           float64
	CorrectIDs      []string
	SelectedIDs     []string
	TextAnswer      string   // numeric questions only
	AcceptedAnswers []string // numeric questions only — option bodies
	PartialCredit   bool
	NegativeMarking float64 // fraction of Marks deducted for a wrong answer
}

// ScoreMcq marks a single MCQ. An unanswered question always scores zero and
// never attracts a negative mark — penalising a blank would punish honest
// skipping, which is not what negative marking is for.
func ScoreMcq(in McqInput) float64 {
	if in.Kind == "numeric" {
		return scoreNumeric(in)
	}

	selected := dedupe(in.SelectedIDs)
	if len(selected) == 0 {
		return 0
	}
	correct := toSet(in.CorrectIDs)

	wrongSelected := 0
	correctSelected := 0
	for _, id := range selected {
		if correct[id] {
			correctSelected++
		} else {
			wrongSelected++
		}
	}

	// Single-answer questions are all-or-nothing regardless of the partial
	// credit flag: there is no meaningful fraction of one correct option.
	if in.Kind == "single" || !in.PartialCredit {
		if wrongSelected == 0 && correctSelected == len(correct) && len(correct) > 0 {
			return in.Marks
		}
		return -in.NegativeMarking * in.Marks
	}

	// Partial credit: any wrong selection voids the question, otherwise award
	// the fraction of correct options found.
	if wrongSelected > 0 {
		return -in.NegativeMarking * in.Marks
	}
	if len(correct) == 0 {
		return 0
	}
	return round2(in.Marks * float64(correctSelected) / float64(len(correct)))
}

// scoreNumeric compares a typed answer against the accepted values. Numbers are
// compared numerically so "2.50" matches "2.5"; anything unparseable falls back
// to a trimmed case-insensitive string match.
func scoreNumeric(in McqInput) float64 {
	answer := strings.TrimSpace(in.TextAnswer)
	if answer == "" {
		return 0
	}
	got, gotErr := strconv.ParseFloat(answer, 64)

	for _, accepted := range in.AcceptedAnswers {
		accepted = strings.TrimSpace(accepted)
		if want, err := strconv.ParseFloat(accepted, 64); err == nil && gotErr == nil {
			if math.Abs(want-got) < 1e-9 {
				return in.Marks
			}
			continue
		}
		if strings.EqualFold(accepted, answer) {
			return in.Marks
		}
	}
	return -in.NegativeMarking * in.Marks
}

// ScoreCoding awards marks in proportion to test cases passed, out of every
// case the problem has (a test submission runs them all). A
// submission that fails to compile passes nothing and so scores zero — coding
// questions never carry a negative mark, since a wrong attempt already costs
// the candidate time.
func ScoreCoding(passed, total int, marks float64) float64 {
	if total <= 0 || passed <= 0 {
		return 0
	}
	if passed >= total {
		return marks
	}
	return round2(marks * float64(passed) / float64(total))
}

// IntegrityPenalty is the deduction applied to an attempt's integrity score for
// one proctoring event. Unknown event kinds cost a small default rather than
// nothing, so a new client-side signal is never silently free.
func IntegrityPenalty(kind string) float64 {
	switch kind {
	case "tab_blur":
		return 5
	case "fullscreen_exit":
		return 8
	case "paste":
		return 10
	case "copy":
		return 3
	case "devtools":
		return 25
	case "multi_face", "no_face":
		return 15
	// Refusing or killing the camera is a decision, not an accident, and it is
	// the one camera signal with no false-positive story — so it costs more
	// than the picture merely looking wrong.
	case "camera_denied", "camera_off":
		return 20
	// A dark frame is usually a covered lens, but sometimes a dim room; a voice
	// is often the candidate reading aloud. Both are worth a reviewer's eye
	// rather than a heavy automatic penalty.
	// Both say "the camera is not showing me a person right now", and both have
	// an innocent reading — a dim room, someone sitting very still. Evidence for
	// a reviewer, not grounds to fail anybody.
	case "camera_dark", "no_motion":
		return 8
	case "voice_detected":
		return 5
	case "disconnect":
		return 2
	// A second session on the same attempt is the test open on another tab or
	// device — the setup for someone else to help, or to sit it for them.
	case "duplicate_session":
		return 20
	case "multi_monitor", "screenshot":
		return 10
	// A new IP mid-test is often innocent (Wi-Fi to hotspot), so it is light.
	case "ip_change":
		return 8
	// Shrinking the window to sit a chatbot beside the test.
	case "window_resized":
		return 3
	default:
		return 2
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func toSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			s[id] = true
		}
	}
	return s
}

func dedupe(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
