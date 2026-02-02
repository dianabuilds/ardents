package net

import (
	"sync"
	"time"
)

type Dedup struct {
	mu      sync.Mutex
	ttl     time.Duration
	seen    map[string]time.Time
	nowFn   func() time.Time
	maxSize int
}

func NewDedup(ttl time.Duration, maxSize int) *Dedup {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &Dedup{
		ttl:     ttl,
		seen:    make(map[string]time.Time),
		nowFn:   time.Now,
		maxSize: maxSize,
	}
}

func (d *Dedup) Seen(msgID string) bool {
	now := d.nowFn()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.gcLocked(now)
	if _, ok := d.seen[msgID]; ok {
		return true
	}
	if len(d.seen) >= d.maxSize {
		d.evictOneLocked()
	}
	d.seen[msgID] = now
	return false
}

func (d *Dedup) gcLocked(now time.Time) {
	exp := now.Add(-d.ttl)
	for k, t := range d.seen {
		if t.Before(exp) {
			delete(d.seen, k)
		}
	}
}

func (d *Dedup) evictOneLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, t := range d.seen {
		if first || t.Before(oldestTime) {
			oldestKey = k
			oldestTime = t
			first = false
		}
	}
	if !first {
		delete(d.seen, oldestKey)
	}
}
