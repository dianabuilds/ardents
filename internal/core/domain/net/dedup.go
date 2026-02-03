package net

import (
	"sync"
	"time"
)

type Dedup struct {
	mu      sync.Mutex
	minTTL  time.Duration
	seen    map[string]time.Time
	nowFn   func() time.Time
	maxSize int
}

func NewDedup(minTTL time.Duration, maxSize int) *Dedup {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &Dedup{
		minTTL:  minTTL,
		seen:    make(map[string]time.Time),
		nowFn:   time.Now,
		maxSize: maxSize,
	}
}

func (d *Dedup) Seen(msgID string) bool {
	return d.SeenWithTTL(msgID, 0)
}

func (d *Dedup) SeenWithTTL(msgID string, ttl time.Duration) bool {
	now := d.nowFn()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.gcLocked(now)
	if exp, ok := d.seen[msgID]; ok {
		if exp.After(now) {
			return true
		}
		delete(d.seen, msgID)
	}
	if ttl < d.minTTL {
		ttl = d.minTTL
	}
	expireAt := now.Add(ttl)
	if len(d.seen) >= d.maxSize {
		d.evictOneLocked()
	}
	d.seen[msgID] = expireAt
	return false
}

func (d *Dedup) SeenUntil(msgID string, expireAt time.Time) bool {
	now := d.nowFn()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.gcLocked(now)
	if exp, ok := d.seen[msgID]; ok {
		if exp.After(now) {
			return true
		}
		delete(d.seen, msgID)
	}
	if len(d.seen) >= d.maxSize {
		d.evictOneLocked()
	}
	d.seen[msgID] = expireAt
	return false
}

func (d *Dedup) gcLocked(now time.Time) {
	for k, t := range d.seen {
		if !t.After(now) {
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
