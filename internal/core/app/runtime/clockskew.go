package runtime

import (
	"sync"
	"time"
)

type clockSkewTracker struct {
	mu         sync.Mutex
	distinct   map[string]bool
	lastSkewed bool
	threshold  int
}

func newClockSkewTracker(threshold int) *clockSkewTracker {
	if threshold <= 0 {
		threshold = 4
	}
	return &clockSkewTracker{
		distinct:  make(map[string]bool),
		threshold: threshold,
	}
}

func (t *clockSkewTracker) Observe(peerID string, skewed bool) (count int, thresholdReached bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !skewed {
		t.distinct = make(map[string]bool)
		t.lastSkewed = false
		return 0, false
	}
	if !t.lastSkewed {
		t.distinct = make(map[string]bool)
	}
	t.lastSkewed = true
	if peerID != "" {
		t.distinct[peerID] = true
	}
	count = len(t.distinct)
	if count >= t.threshold {
		return count, true
	}
	return count, false
}

func skewedNow(localNowMs, remoteTSMs int64) bool {
	if localNowMs <= 0 || remoteTSMs <= 0 {
		return false
	}
	diff := time.Duration(localNowMs-remoteTSMs) * time.Millisecond
	if diff < 0 {
		diff = -diff
	}
	return diff > 5*time.Minute
}
