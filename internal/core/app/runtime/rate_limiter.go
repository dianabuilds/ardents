package runtime

import "sync"

type rateLimiter struct {
	limit    uint64
	windowMs int64
	mu       sync.Mutex
	buckets  map[string]rateBucket
}

type rateBucket struct {
	resetAtMs int64
	count     uint64
}

func newRateLimiter(limit uint64, windowMs int64) *rateLimiter {
	if limit == 0 || windowMs <= 0 {
		return nil
	}
	return &rateLimiter{
		limit:    limit,
		windowMs: windowMs,
		buckets:  make(map[string]rateBucket),
	}
}

func (r *rateLimiter) Allow(key string, nowMs int64) bool {
	if r == nil {
		return true
	}
	if key == "" {
		key = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.buckets[key]
	if !ok || nowMs >= b.resetAtMs {
		r.buckets[key] = rateBucket{
			resetAtMs: nowMs + r.windowMs,
			count:     1,
		}
		return true
	}
	if b.count >= r.limit {
		return false
	}
	b.count++
	r.buckets[key] = b
	return true
}
