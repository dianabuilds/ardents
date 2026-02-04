package runtime

type powAbuseTracker struct {
	counter *thresholdCounter
}

func newPowAbuseTracker(threshold int) *powAbuseTracker {
	return &powAbuseTracker{counter: newThresholdCounter(threshold)}
}

func (t *powAbuseTracker) Inc(peerID string) (count int, reached bool) {
	if t == nil {
		return 0, false
	}
	return t.counter.Inc(peerID)
}

func (t *powAbuseTracker) Reset(peerID string) {
	if t == nil {
		return
	}
	t.counter.Reset(peerID)
}
