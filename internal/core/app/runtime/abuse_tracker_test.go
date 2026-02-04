package runtime

import "testing"

func TestThresholdCounter_DefaultThresholdAndReset(t *testing.T) {
	c := newThresholdCounter(0) // default to 5
	key := "peer_x"

	for i := 1; i <= 4; i++ {
		n, reached := c.Inc(key)
		if n != i {
			t.Fatalf("count mismatch: got=%d want=%d", n, i)
		}
		if reached {
			t.Fatalf("reached too early at i=%d", i)
		}
	}
	n, reached := c.Inc(key)
	if n != 5 || !reached {
		t.Fatalf("expected reached at 5: n=%d reached=%v", n, reached)
	}

	c.Reset(key)
	n, reached = c.Inc(key)
	if n != 1 || reached {
		t.Fatalf("expected reset: n=%d reached=%v", n, reached)
	}
}

func TestAbuseTrackers_AreIndependentAndUseSharedCounter(t *testing.T) {
	powT := newPowAbuseTracker(2)
	hsT := newHandshakeAbuseTracker(2)

	if _, reached := powT.Inc("peer_a"); reached {
		t.Fatalf("pow reached too early")
	}
	if _, reached := hsT.Inc("peer_a"); reached {
		t.Fatalf("handshake reached too early")
	}

	if _, reached := powT.Inc("peer_a"); !reached {
		t.Fatalf("pow expected reached")
	}
	// handshake is independent: second inc reaches for its own counter.
	if _, reached := hsT.Inc("peer_a"); !reached {
		t.Fatalf("handshake expected reached")
	}
}
