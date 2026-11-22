package resolvers

import (
	"errors"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// runGate lets each learner have one code run in flight at a time.
//
// Run is synchronous and the runners are a small shared pool. A learner who
// clicks Run again while the first is still going only queues a second run
// behind it — and, under load, behind everyone else — so the repeat is refused
// with a message instead. In memory per gateway instance; with several
// gateways this would move to Redis.
type runGate struct {
	mu       sync.Mutex
	inFlight map[string]bool
}

var learnerRuns = &runGate{inFlight: map[string]bool{}}

var errRunInFlight = errors.New("Your previous run is still going. Wait for it to finish, then try again.")

// acquire claims the learner's run slot; release must be called when it is true.
func (g *runGate) acquire(userID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight[userID] {
		return false
	}
	g.inFlight[userID] = true
	return true
}

func (g *runGate) release(userID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inFlight, userID)
}

// runErr turns a failed run into what the learner sees: "busy", "not found" and
// input problems keep their own (learner-worded) message; anything else — a
// container or network failure — gets a plain retry message rather than an
// internal error string.
func runErr(err error, fallback string) error {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.ResourceExhausted, codes.NotFound, codes.InvalidArgument:
			if st.Message() != "" {
				return errors.New(st.Message())
			}
		}
	}
	return errors.New(fallback)
}
