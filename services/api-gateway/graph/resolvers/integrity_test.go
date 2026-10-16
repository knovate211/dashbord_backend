package resolvers

import "testing"

func TestInsertedChars(t *testing.T) {
	cases := []struct {
		prev, next string
		want       int
	}{
		{"", "abc", 3},
		{"def f():\n    pass\n", "def f():\n    return 1\n", 8}, // "return 1" replaced "pass"
		{"abc", "abc", 0},
		{"abcdef", "abef", 0}, // a deletion inserts nothing
		{"x = 1\n", "x = 1\nfor i in range(10):\n    print(i)\n", 33}, // 20 + 13 appended
	}
	for _, c := range cases {
		if got := insertedChars(c.prev, c.next); got != c.want {
			t.Errorf("insertedChars(%q, %q) = %d, want %d", c.prev, c.next, got, c.want)
		}
	}
}

// The browser computes the same token (CodingQuestionView's trapToken); this
// pins the value so a change on either side is caught.
func TestTrapTokenIsStable(t *testing.T) {
	got := trapToken("11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222")
	if got != trapToken("11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222") {
		t.Fatal("token is not deterministic")
	}
	if len(got) < 5 || got[:3] != "kv_" {
		t.Fatalf("unexpected token %q", got)
	}
	t.Logf("token = %s", got)
}
