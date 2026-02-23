package daemonservice

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type bandwidthLimiter struct {
	mu      sync.RWMutex
	limit   int
	limiter *rate.Limiter
}

func newBandwidthLimiter(kbps int) *bandwidthLimiter {
	b := &bandwidthLimiter{}
	b.SetLimitKBps(kbps)
	return b
}

func (b *bandwidthLimiter) SetLimitKBps(kbps int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if kbps <= 0 {
		b.limit = 0
		b.limiter = nil
		return
	}
	bytesPerSecond := kbps * 1024
	if bytesPerSecond < 1 {
		bytesPerSecond = 1
	}
	b.limit = kbps
	b.limiter = rate.NewLimiter(rate.Limit(bytesPerSecond), bytesPerSecond)
}

func (b *bandwidthLimiter) AllowBytes(bytes int) bool {
	if b == nil || bytes <= 0 {
		return true
	}
	b.mu.RLock()
	limiter := b.limiter
	b.mu.RUnlock()
	if limiter == nil {
		return true
	}
	return limiter.AllowN(time.Now(), bytes)
}

func (b *bandwidthLimiter) LimitKBps() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.limit
}
