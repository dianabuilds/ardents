package runtime

import "sync"

// thresholdCounter tracks per-key counts and reports when a threshold is reached.
// It is used for abuse/anti-spam heuristics (PoW invalid, handshake errors, etc.).
type thresholdCounter struct {
	mu        sync.Mutex
	counts    map[string]int
	threshold int
}

func newThresholdCounter(threshold int) *thresholdCounter {
	if threshold <= 0 {
		threshold = 5
	}
	return &thresholdCounter{
		counts:    make(map[string]int),
		threshold: threshold,
	}
}

func (t *thresholdCounter) Inc(key string) (count int, reached bool) {
	if t == nil || key == "" {
		return 0, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts[key]++
	count = t.counts[key]
	return count, count >= t.threshold
}

func (t *thresholdCounter) Reset(key string) {
	if t == nil || key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.counts, key)
}
