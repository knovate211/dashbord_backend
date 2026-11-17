package resolvers

import (
	"strings"
	"testing"
)

func TestNewJoinCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := newJoinCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 9 || c[4] != '-' {
			t.Fatalf("code %q is not ABCD-EFGH", c)
		}
		for _, r := range strings.ReplaceAll(c, "-", "") {
			if !strings.ContainsRune(joinCodeAlphabet, r) {
				t.Fatalf("code %q uses %q, outside the alphabet", c, r)
			}
		}
		if normaliseJoinCode(c) != c {
			t.Fatalf("normaliseJoinCode(%q) changed a valid code", c)
		}
		seen[c] = true
	}
	if len(seen) < 195 {
		t.Fatalf("only %d distinct codes in 200", len(seen))
	}
}

func TestNormaliseJoinCode(t *testing.T) {
	cases := map[string]string{
		"abcd-efgh":   "ABCD-EFGH",
		" ABCD EFGH ": "ABCD-EFGH",
		"ABCDEFGH":    "ABCD-EFGH",
		"ABCD-EFG":    "",
		"ABCD-EFGHJ":  "",
		"":            "",
		// 0, O, 1, I and L are not in the alphabet, so they never match.
		"ABC0-EFGH": "",
	}
	for in, want := range cases {
		if got := normaliseJoinCode(in); got != want {
			t.Errorf("normaliseJoinCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCSVSafe(t *testing.T) {
	got := csvSafeRow([]string{"=HYPERLINK(\"x\")", "+1", "-2", "@cmd", "Asha", ""})
	want := []string{"'=HYPERLINK(\"x\")", "'+1", "'-2", "'@cmd", "Asha", ""}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("csvSafe[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCSVFileName(t *testing.T) {
	cases := map[string]string{
		"R.V. College of Engineering, Bengaluru": "r-v-college-of-engineering-bengaluru-students.csv",
		"!!!":                                    "college-students.csv",
	}
	for in, want := range cases {
		if got := csvFileName(in); got != want {
			t.Errorf("csvFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCourseTotalsPositive(t *testing.T) {
	for key, c := range courseTotals {
		if c.Total <= 0 || c.Title == "" {
			t.Errorf("course %s has total %d, title %q", key, c.Total, c.Title)
		}
	}
}
