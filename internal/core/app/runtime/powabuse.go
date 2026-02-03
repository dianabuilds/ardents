package runtime

import "sync"

type powAbuseTracker struct {
	mu        sync.Mutex
	counts    map[string]int
	threshold int
}

func newPowAbuseTracker(threshold int) *powAbuseTracker {
	if threshold <= 0 {
		threshold = 5
	}
	return &powAbuseTracker{
		counts:    make(map[string]int),
		threshold: threshold,
	}
}

func (t *powAbuseTracker) Inc(peerID string) (count int, reached bool) {
	if peerID == "" {
		return 0, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts[peerID]++
	count = t.counts[peerID]
	if count >= t.threshold {
		return count, true
	}
	return count, false
}

func (t *powAbuseTracker) Reset(peerID string) {
	if peerID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.counts, peerID)
}
