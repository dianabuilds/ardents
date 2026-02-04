package runtime

type handshakeAbuseTracker struct {
	counter *thresholdCounter
}

func newHandshakeAbuseTracker(threshold int) *handshakeAbuseTracker {
	return &handshakeAbuseTracker{counter: newThresholdCounter(threshold)}
}

func (t *handshakeAbuseTracker) Inc(peerID string) (count int, reached bool) {
	if t == nil {
		return 0, false
	}
	return t.counter.Inc(peerID)
}

func (t *handshakeAbuseTracker) Reset(peerID string) {
	if t == nil {
		return
	}
	t.counter.Reset(peerID)
}
