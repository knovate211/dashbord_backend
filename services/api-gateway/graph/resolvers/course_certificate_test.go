package resolvers

import "testing"

func TestCourseComplete(t *testing.T) {
	java := courseTotals["maxJavaIndexRead"].Total
	cases := []struct {
		key  string
		idx  int
		want bool
	}{
		{"maxJavaIndexRead", java - 1, true}, // last item read
		{"maxJavaIndexRead", java + 5, true},
		{"maxJavaIndexRead", java - 2, false},
		{"maxJavaIndexRead", 0, false},
		{"unknownCourse", 9999, false},
	}
	for _, c := range cases {
		if got := courseComplete(c.key, c.idx); got != c.want {
			t.Errorf("courseComplete(%q, %d) = %v, want %v", c.key, c.idx, got, c.want)
		}
	}
	if title, ok := courseCertificateTitle("maxJavaIndexRead"); !ok || title != "Java Development" {
		t.Errorf("title = %q, %v", title, ok)
	}
}
