package runtime

import "sync"

type handshakeAbuseTracker struct {
	mu        sync.Mutex
	counts    map[string]int
	threshold int
}

func newHandshakeAbuseTracker(threshold int) *handshakeAbuseTracker {
	if threshold <= 0 {
		threshold = 5
	}
	return &handshakeAbuseTracker{
		counts:    make(map[string]int),
		threshold: threshold,
	}
}

func (t *handshakeAbuseTracker) Inc(peerID string) (count int, reached bool) {
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

func (t *handshakeAbuseTracker) Reset(peerID string) {
	if peerID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.counts, peerID)
}
