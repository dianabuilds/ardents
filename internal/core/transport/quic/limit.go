package quic

import (
	"errors"
	"net"
	"sync"
	"time"
)

var (
	ErrMaxInboundConns      = errors.New("ERR_MAX_INBOUND_CONNS")
	ErrMaxOutboundConns     = errors.New("ERR_MAX_OUTBOUND_CONNS")
	ErrHandshakeRateLimited = errors.New("ERR_HANDSHAKE_RATE_LIMITED")
	ErrPeerBanned           = errors.New("ERR_PEER_BANNED")
)

type attemptLimiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	hits   map[string][]time.Time
	nowFn  func() time.Time
}

func newAttemptLimiter(max int, window time.Duration) *attemptLimiter {
	if max <= 0 || window <= 0 {
		return nil
	}
	return &attemptLimiter{
		window: window,
		max:    max,
		hits:   make(map[string][]time.Time),
		nowFn:  time.Now,
	}
}

func (l *attemptLimiter) Allow(key string) bool {
	if l == nil || key == "" {
		return true
	}
	now := l.nowFn()
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	entries := l.hits[key]
	if len(entries) > 0 {
		n := 0
		for _, t := range entries {
			if t.After(cutoff) {
				entries[n] = t
				n++
			}
		}
		entries = entries[:n]
	}
	if len(entries) >= l.max {
		l.hits[key] = entries
		return false
	}
	entries = append(entries, now)
	l.hits[key] = entries
	return true
}

func normalizeAddrKey(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// errorsNew is a tiny wrapper to keep error vars close without extra imports.
