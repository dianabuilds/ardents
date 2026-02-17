package ratelimiter

import (
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// MapLimiter applies a token bucket per string key and periodically evicts idle entries.
type MapLimiter struct {
	limit   rate.Limit
	burst   int
	mu      sync.Mutex
	byKey   map[string]*entry
	hits    uint64
	idleTTL time.Duration
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// New creates a key-based limiter; returns nil if args are invalid.
func New(rps float64, burst int, idleTTL time.Duration) *MapLimiter {
	if rps <= 0 || burst <= 0 {
		return nil
	}
	if idleTTL <= 0 {
		idleTTL = 10 * time.Minute
	}
	return &MapLimiter{
		limit:   rate.Limit(rps),
		burst:   burst,
		byKey:   make(map[string]*entry),
		idleTTL: idleTTL,
	}
}

// Allow reports whether one token can be consumed for the key at now.
func (l *MapLimiter) Allow(key string, now time.Time) bool {
	if l == nil {
		return true
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.byKey[key]
	if !ok {
		e = &entry{
			limiter:  rate.NewLimiter(l.limit, l.burst),
			lastSeen: now,
		}
		l.byKey[key] = e
	}
	e.lastSeen = now
	allowed := e.limiter.AllowN(now, 1)

	l.hits++
	if l.hits%512 == 0 {
		cutoff := now.Add(-l.idleTTL)
		for k, v := range l.byKey {
			if v.lastSeen.Before(cutoff) {
				delete(l.byKey, k)
			}
		}
	}

	return allowed
}
