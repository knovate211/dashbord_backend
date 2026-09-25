package resolvers

import "testing"

const simOriginal = `
def two_sum(nums, target):
    # remember where each value was seen
    seen = {}
    for i, n in enumerate(nums):
        want = target - n
        if want in seen:
            return [seen[want], i]
        seen[n] = i
    return []
`

// The same solution with renamed variables, new comments and reformatting —
// the classic disguise. It must still be caught.
const simDisguised = `
def two_sum(arr, goal):
    """Find the pair."""
    lookup = {}          # value -> index
    for idx, val in enumerate(arr):
        need = goal - val
        if need in lookup:
            return [lookup[need], idx]
        lookup[val] = idx
    return []
`

// A genuinely different correct approach to the same problem.
const simIndependent = `
def two_sum(nums, target):
    order = sorted(range(len(nums)), key=lambda k: nums[k])
    lo, hi = 0, len(nums) - 1
    while lo < hi:
        total = nums[order[lo]] + nums[order[hi]]
        if total == target:
            return sorted([order[lo], order[hi]])
        if total < target:
            lo += 1
        else:
            hi -= 1
    return []
`

func TestSimilarityCatchesRenamedCopy(t *testing.T) {
	a := simFingerprint(simOriginal, "python")
	b := simFingerprint(simDisguised, "python")
	pct, ok := simScore(a, b)
	if !ok || pct < 90 {
		t.Fatalf("renamed copy scored %d%% (ok=%v), want >= 90%%", pct, ok)
	}
}

func TestSimilaritySeparatesIndependentSolutions(t *testing.T) {
	a := simFingerprint(simOriginal, "python")
	b := simFingerprint(simIndependent, "python")
	pct, ok := simScore(a, b)
	if !ok {
		t.Fatal("both solutions are long enough to compare")
	}
	if pct > 40 {
		t.Fatalf("independent solutions scored %d%%, want <= 40%%", pct)
	}
}

func TestSimilarityIgnoresStarterBoilerplate(t *testing.T) {
	starter := "import java.util.*;\npublic class Solution {\n    public int[] twoSum(int[] nums, int target) {\n        // your code here\n        return new int[0];\n    }\n}\n"
	// Two candidates who wrote almost nothing beyond the starter must not be
	// reported as copying each other just because the template matches.
	a := starter
	b := starter + "\n"
	base := simFingerprint(starter, "java")
	pct, ok := simScore(simFingerprint(a, "java").without(base), simFingerprint(b, "java").without(base))
	if ok && pct > 0 {
		t.Fatalf("starter-only code scored %d%%; boilerplate should be excluded", pct)
	}
}

func TestSimilarityTooShortToJudge(t *testing.T) {
	if _, ok := simScore(simFingerprint("return a+b", "python"), simFingerprint("return a+b", "python")); ok {
		t.Fatal("a one-line answer is too short to call a copy")
	}
}

func TestSimTokensDropCommentsAndNormaliseNames(t *testing.T) {
	got := simTokens("x = 1 // note\n/* block */ y = \"s\"", "javascript")
	want := []string{"I", "=", "N", "I", "=", "S"}
	if len(got) != len(want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokens = %v, want %v", got, want)
		}
	}
}
