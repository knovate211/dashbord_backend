package resolvers

import "testing"

// Payment verification is covered in cashfree_test.go.

func TestFeeFor(t *testing.T) {
	cases := []struct {
		course, plan string
		want         int
		ok           bool
	}{
		{"5", "self", 10000, true},
		{"1", "mentor", 10000, true},
		{"testing", "self", 10000, true},
		{"digital-marketing", "mentor", 10000, true},
		{"genai", "mentor", 10000, true},
		{"3", "self", 10000, true},
		{"5", "gold", 0, false},
		{"nope", "self", 0, false},
	}
	for _, c := range cases {
		got, _, ok := feeFor(c.course, c.plan)
		if got != c.want || ok != c.ok {
			t.Errorf("feeFor(%q,%q) = %d,%v want %d,%v", c.course, c.plan, got, ok, c.want, c.ok)
		}
	}
}

func TestGeneratePassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, err := generatePassword()
		if err != nil || len(p) != 12 || seen[p] {
			t.Fatalf("bad password %q err=%v", p, err)
		}
		seen[p] = true
	}
}
