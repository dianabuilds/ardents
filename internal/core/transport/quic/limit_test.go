package quic

import (
	"testing"
	"time"
)

func TestAttemptLimiter(t *testing.T) {
	lim := newAttemptLimiter(2, time.Minute)
	if lim == nil {
		t.Fatal("expected limiter")
	}
	now := time.Unix(0, 0)
	lim.nowFn = func() time.Time { return now }

	if !lim.Allow("a") {
		t.Fatal("expected first allow")
	}
	if !lim.Allow("a") {
		t.Fatal("expected second allow")
	}
	if lim.Allow("a") {
		t.Fatal("expected rate limit")
	}

	now = now.Add(2 * time.Minute)
	if !lim.Allow("a") {
		t.Fatal("expected allow after window")
	}
}
