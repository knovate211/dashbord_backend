package repository

import (
	"reflect"
	"testing"
)

func TestPreferUnseen(t *testing.T) {
	seen := map[string]bool{"b": true, "d": true}
	is := func(s string) bool { return seen[s] }

	ids := []string{"a", "b", "c", "d", "e"}
	preferUnseen(ids, is)
	if want := []string{"a", "c", "e", "b", "d"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("unseen first, order kept: got %v want %v", ids, want)
	}

	// Too few unseen: the draw still fills from seen questions.
	ids = []string{"b", "d", "a"}
	preferUnseen(ids, is)
	if got := ids[:2]; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("shortfall filled from seen: got %v", got)
	}
}
