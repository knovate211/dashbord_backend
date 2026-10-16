package resolvers

import (
	"testing"
	"time"
)

func TestRateLimiterZeroMeansNoLimit(t *testing.T) {
	l := newRateLimiter(0, time.Minute)
	if l != nil {
		t.Fatal("limit 0 should give a nil (disabled) limiter")
	}
	for i := 0; i < 10000; i++ {
		if !l.allow("10.0.0.1") {
			t.Fatalf("disabled limiter refused request %d", i)
		}
	}
}

func TestRateLimiterCapsPerKey(t *testing.T) {
	l := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.allow("a@example.com") {
			t.Fatalf("request %d refused under the cap", i)
		}
	}
	if l.allow("a@example.com") {
		t.Fatal("4th request allowed over a cap of 3")
	}
	if !l.allow("b@example.com") {
		t.Fatal("a different key was blocked by another key's usage")
	}
}

func TestDirectResetKeepsIPCap(t *testing.T) {
	if directDefault(true) <= 0 {
		t.Fatal("unverified direct reset must keep an IP cap by default")
	}
	if directDefault(false) != 0 {
		t.Fatal("code-verified reset should have no IP cap by default")
	}
}
