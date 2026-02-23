package manifestruntime

import (
	"math/rand"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		RefreshInterval:      60 * time.Second,
		StaleRefreshInterval: 15 * time.Second,
		SlowPollingInterval:  60 * time.Second,
		StaleWindow:          5 * time.Minute,
		BackoffBase:          1 * time.Second,
		BackoffMax:           5 * time.Second,
		BackoffFactor:        2.0,
		BackoffJitterRatio:   0,
	}
}

func TestStateAt(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	expires := now.Add(10 * time.Minute)

	if got := StateAt(now, expires, 5*time.Minute); got != StateFresh {
		t.Fatalf("expected fresh, got %s", got)
	}
	if got := StateAt(now.Add(7*time.Minute), expires, 5*time.Minute); got != StateStale {
		t.Fatalf("expected stale, got %s", got)
	}
	if got := StateAt(now.Add(10*time.Minute), expires, 5*time.Minute); got != StateExpired {
		t.Fatalf("expected expired, got %s", got)
	}
}

func TestDecideRecoverableErrorUsesBackoffAndFallback(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	rnd := rand.New(rand.NewSource(1))
	c := NewController(testConfig(), rnd)

	first := c.Decide(now, AttemptOutcome{
		ErrorKind:   ErrorRecoverable,
		CacheUsable: true,
	})
	if first.SourceAfter != SourceCache {
		t.Fatalf("expected source cache, got %s", first.SourceAfter)
	}
	if first.NextDelay != 1*time.Second {
		t.Fatalf("expected first backoff 1s, got %s", first.NextDelay)
	}

	second := c.Decide(now.Add(1*time.Second), AttemptOutcome{
		ErrorKind:   ErrorRecoverable,
		CacheUsable: true,
	})
	if second.NextDelay != 2*time.Second {
		t.Fatalf("expected second backoff 2s, got %s", second.NextDelay)
	}
}

func TestDecideExpiredFallsBackToBaked(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	c := NewController(testConfig(), rand.New(rand.NewSource(1)))

	accepted := c.Decide(now, AttemptOutcome{
		ManifestAccepted:  true,
		ManifestExpiresAt: now.Add(5 * time.Second),
	})
	if accepted.SourceAfter != SourceManifest {
		t.Fatalf("expected source manifest, got %s", accepted.SourceAfter)
	}

	fallback := c.Decide(now.Add(10*time.Second), AttemptOutcome{
		ErrorKind:   ErrorNonRecoverable,
		BakedUsable: true,
	})
	if fallback.State != StateExpired {
		t.Fatalf("expected expired state, got %s", fallback.State)
	}
	if fallback.SourceAfter != SourceBaked {
		t.Fatalf("expected source baked, got %s", fallback.SourceAfter)
	}
	if fallback.NextDelay != 60*time.Second {
		t.Fatalf("expected slow polling 60s, got %s", fallback.NextDelay)
	}
}

func TestDecideRestoresManifestAfterFallback(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	c := NewController(testConfig(), rand.New(rand.NewSource(1)))

	c.Decide(now, AttemptOutcome{
		ErrorKind:   ErrorRecoverable,
		CacheUsable: true,
	})
	if c.Source() != SourceCache {
		t.Fatalf("expected source cache before restore, got %s", c.Source())
	}

	restore := c.Decide(now.Add(2*time.Second), AttemptOutcome{
		ManifestAccepted:  true,
		ManifestExpiresAt: now.Add(10 * time.Minute),
	})
	if restore.SourceAfter != SourceManifest {
		t.Fatalf("expected source manifest after restore, got %s", restore.SourceAfter)
	}
	if !restore.RestoredManifest {
		t.Fatal("expected restored manifest flag")
	}
}

func TestDecideNonRecoverableResetsBackoffSequence(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	c := NewController(testConfig(), rand.New(rand.NewSource(1)))

	c.Decide(now, AttemptOutcome{
		ErrorKind:   ErrorRecoverable,
		CacheUsable: true,
	})
	c.Decide(now.Add(1*time.Second), AttemptOutcome{
		ErrorKind:   ErrorNonRecoverable,
		BakedUsable: true,
	})
	next := c.Decide(now.Add(2*time.Second), AttemptOutcome{
		ErrorKind:   ErrorRecoverable,
		CacheUsable: true,
	})
	if next.NextDelay != 1*time.Second {
		t.Fatalf("expected backoff sequence reset to 1s, got %s", next.NextDelay)
	}
}

func TestSelectFallbackSource(t *testing.T) {
	if got, err := SelectFallbackSource(true, true); err != nil || got != SourceCache {
		t.Fatalf("expected cache fallback, got source=%s err=%v", got, err)
	}
	if got, err := SelectFallbackSource(false, true); err != nil || got != SourceBaked {
		t.Fatalf("expected baked fallback, got source=%s err=%v", got, err)
	}
	if got, err := SelectFallbackSource(false, false); err == nil || got != SourceNone {
		t.Fatalf("expected none+error fallback, got source=%s err=%v", got, err)
	}
}
