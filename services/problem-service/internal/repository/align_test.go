package repository

import (
	"testing"

	problemv1 "github.com/knovate211/proto/problem/v1"
)

func TestAlignExamplesToTestCases(t *testing.T) {
	authored := []*problemv1.Example{
		{Input: "nums = [2,7], target = 9", Output: "[0,1]", Explanation: "2 + 7 = 9"},
		{Input: "stale", Output: "stale", Explanation: "second"},
		{Input: "never run", Output: "x", Explanation: "dropped"},
	}
	cases := []visibleCase{{Input: "[2,7]\n9", Expected: "[0,1]"}, {Input: "[3,3]\n6", Expected: "[0,1]"}}

	got := alignExamplesToTestCases(authored, cases, "function")
	if len(got) != 2 {
		t.Fatalf("want one example per visible case, got %d", len(got))
	}
	if got[0].Input != "[2,7]\n9" || got[0].Output != "[0,1]" || got[0].Explanation != "2 + 7 = 9" {
		t.Fatalf("first example = %+v", got[0])
	}
	if got[1].Input != "[3,3]\n6" || got[1].Explanation != "second" {
		t.Fatalf("second example = %+v", got[1])
	}

	// Only function-mode problems are realigned, and nothing changes without cases.
	if out := alignExamplesToTestCases(authored, cases, "stdio"); len(out) != 3 || out[0] != authored[0] {
		t.Fatalf("stdio problem was realigned")
	}
	if out := alignExamplesToTestCases(authored, nil, "function"); len(out) != 3 {
		t.Fatalf("problem without visible cases was realigned")
	}
	if out := alignExamplesToTestCases(nil, cases, "function"); len(out) != 0 {
		t.Fatalf("problem without authored examples grew some")
	}
}
