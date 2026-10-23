package resolvers

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRunGateOnePerLearner(t *testing.T) {
	g := &runGate{inFlight: map[string]bool{}}
	if !g.acquire("a") {
		t.Fatal("first run refused")
	}
	if g.acquire("a") {
		t.Fatal("second concurrent run for the same learner was allowed")
	}
	if !g.acquire("b") {
		t.Fatal("another learner was blocked")
	}
	g.release("a")
	if !g.acquire("a") {
		t.Fatal("run refused after the previous one finished")
	}
}

func TestRunErrKeepsOnlyLearnerMessages(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{status.Error(codes.ResourceExhausted, "busy, try again"), "busy, try again"},
		{status.Error(codes.NotFound, "problem not found: x"), "problem not found: x"},
		{status.Error(codes.Internal, "create container: docker socket gone"), "fallback"},
		{errors.New("dial tcp: refused"), "fallback"},
	}
	for _, c := range cases {
		if got := runErr(c.err, "fallback").Error(); got != c.want {
			t.Errorf("runErr(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
