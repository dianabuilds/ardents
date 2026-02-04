package discoverycache

import "sync"

// AddLimiter rate-limits adding new cache entries to mitigate spam.
type AddLimiter struct {
	limit    uint64
	windowMs int64
	mu       sync.Mutex
	resetAt  int64
	count    uint64
}

func NewAddLimiter(limit uint64, windowMs int64) *AddLimiter {
	if limit == 0 || windowMs <= 0 {
		return nil
	}
	return &AddLimiter{limit: limit, windowMs: windowMs}
}

func (l *AddLimiter) Allow(nowMs int64) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.resetAt == 0 || nowMs >= l.resetAt {
		l.resetAt = nowMs + l.windowMs
		l.count = 1
		return true
	}
	if l.count >= l.limit {
		return false
	}
	l.count++
	return true
}
