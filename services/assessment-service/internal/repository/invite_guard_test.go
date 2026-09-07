package repository

import (
	"context"
	"testing"

	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// TestInviteOnly pins down who may sit which kind of paper.
//
// This is a regression test for a real hole rather than a restatement of the
// implementation: resolveInvite used to compare against "hiring" alone, so
// introducing scholarship papers with that comparison intact would have let any
// authenticated student start one without an invitation — and a scholarship is
// awarded on the result.
func TestInviteOnly(t *testing.T) {
	tests := []struct {
		purpose string
		want    bool
		why     string
	}{
		{"practice", false, "practice tests are open to every signed-in student by design"},
		{"hiring", true, "a company's drive is only for the candidates it invited"},
		{"scholarship", true, "the invite row is the entire eligibility record for a scholarship paper"},
		{"", true, "an unknown purpose must fail closed, not open"},
		{"Scholarship", true, "case must not be a way past the guard"},
	}

	for _, tc := range tests {
		t.Run(tc.purpose, func(t *testing.T) {
			if got := inviteOnly(tc.purpose); got != tc.want {
				t.Errorf("inviteOnly(%q) = %v, want %v — %s", tc.purpose, got, tc.want, tc.why)
			}
		})
	}
}

// TestResolveInviteSkipsOnlyPractice checks the guard from the caller's side.
//
// resolveInvite returns early — before touching the database — only for a
// purpose that needs no invitation. Driving it with a nil pool makes that
// difference observable without a live Postgres: the practice case must return
// cleanly, and an invite-only purpose must NOT reach the early return, because
// reaching it is exactly the bug.
func TestResolveInviteSkipsOnlyPractice(t *testing.T) {
	r := &Repo{} // nil pool: any query would panic, which is the point

	t.Run("practice returns no invite and never queries", func(t *testing.T) {
		id, err := r.resolveInvite(context.Background(),
			&assessmentv1.Assessment{Purpose: "practice"},
			&assessmentv1.StartAttemptRequest{UserId: "u1", AssessmentId: "a1"})
		if err != nil {
			t.Fatalf("practice test should need no invite, got error: %v", err)
		}
		if id != "" {
			t.Fatalf("practice test should resolve no invite id, got %q", id)
		}
	})

	for _, purpose := range []string{"hiring", "scholarship"} {
		t.Run(purpose+" must look the invite up", func(t *testing.T) {
			// A nil pool means the lookup panics rather than returning. That is
			// the signal we want: it proves control flow got past the guard and
			// into the invite check. If this ever stops panicking and returns
			// ("", nil) instead, the guard has been widened by mistake and an
			// uninvited candidate can start the paper.
			defer func() {
				if recover() == nil {
					t.Fatalf("%s paper returned without checking for an invite — "+
						"uninvited candidates can start it", purpose)
				}
			}()
			id, err := r.resolveInvite(context.Background(),
				&assessmentv1.Assessment{Purpose: purpose},
				&assessmentv1.StartAttemptRequest{UserId: "u1", AssessmentId: "a1"})
			t.Fatalf("expected the invite lookup to run; got id=%q err=%v", id, err)
		})
	}
}
