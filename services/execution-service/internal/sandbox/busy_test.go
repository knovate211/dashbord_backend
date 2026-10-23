package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

// With every slot taken, an interactive run (MaxQueueWait set) is turned away
// promptly with ErrBusy; a queued submission (no limit) keeps waiting until its
// context ends. Neither reaches Docker, so no daemon is needed.
func TestRunQueueWait(t *testing.T) {
	s := &DockerSandbox{slots: make(chan struct{}, 1)}
	s.slots <- struct{}{} // the only slot is busy

	start := time.Now()
	_, err := s.Run(context.Background(), &RunRequest{Language: "python", Code: "print(1)", MaxQueueWait: 50 * time.Millisecond})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("interactive run: got %v, want ErrBusy", err)
	}
	if waited := time.Since(start); waited > time.Second {
		t.Fatalf("interactive run waited %v", waited)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = s.Run(ctx, &RunRequest{Language: "python", Code: "print(1)"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued run: got %v, want it to wait until its context ended", err)
	}
}
