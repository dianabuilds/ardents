package daemonservice

import (
	"strings"
	"sync"
	"time"

	"aim-chat/go-backend/pkg/models"
)

type publicEphemeralBlobCache struct {
	mu       sync.Mutex
	ttl      time.Duration
	maxBytes int64
	used     int64
	items    map[string]publicEphemeralBlobEntry
}

type publicEphemeralBlobEntry struct {
	meta       models.AttachmentMeta
	data       []byte
	size       int64
	expiresAt  time.Time
	lastAccess time.Time
}

func newPublicEphemeralBlobCache(maxMB int, ttlMinutes int) *publicEphemeralBlobCache {
	cache := &publicEphemeralBlobCache{
		items: map[string]publicEphemeralBlobEntry{},
	}
	cache.Configure(maxMB, ttlMinutes)
	return cache
}

func (c *publicEphemeralBlobCache) Configure(maxMB int, ttlMinutes int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxBytes = int64(maxMB) * 1024 * 1024
	if maxMB <= 0 {
		c.maxBytes = 0
	}
	c.ttl = time.Duration(ttlMinutes) * time.Minute
	if ttlMinutes <= 0 {
		c.ttl = 0
	}
	if c.maxBytes <= 0 || c.ttl <= 0 {
		c.items = map[string]publicEphemeralBlobEntry{}
		c.used = 0
		return
	}
	c.pruneExpiredLocked(time.Now().UTC())
	c.enforceBudgetLocked()
}

func (c *publicEphemeralBlobCache) Put(meta models.AttachmentMeta, data []byte, now time.Time) bool {
	if c == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	id := strings.TrimSpace(meta.ID)
	if id == "" || len(data) == 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxBytes <= 0 || c.ttl <= 0 {
		return false
	}
	c.pruneExpiredLocked(now)
	size := int64(len(data))
	if size > c.maxBytes {
		return false
	}
	if existing, ok := c.items[id]; ok {
		c.used -= existing.size
		delete(c.items, id)
	}
	c.items[id] = publicEphemeralBlobEntry{
		meta:       meta,
		data:       append([]byte(nil), data...),
		size:       size,
		expiresAt:  now.Add(c.ttl),
		lastAccess: now,
	}
	c.used += size
	c.enforceBudgetLocked()
	return true
}

func (c *publicEphemeralBlobCache) Get(id string, now time.Time) (models.AttachmentMeta, []byte, bool) {
	if c == nil {
		return models.AttachmentMeta{}, nil, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return models.AttachmentMeta{}, nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked(now)
	entry, ok := c.items[id]
	if !ok {
		return models.AttachmentMeta{}, nil, false
	}
	entry.lastAccess = now
	entry.expiresAt = now.Add(c.ttl)
	c.items[id] = entry
	return entry.meta, append([]byte(nil), entry.data...), true
}

func (c *publicEphemeralBlobCache) PurgeExpired(now time.Time) {
	if c == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked(now)
}

func (c *publicEphemeralBlobCache) pruneExpiredLocked(now time.Time) {
	if len(c.items) == 0 {
		return
	}
	for id, entry := range c.items {
		if !entry.expiresAt.After(now) {
			c.used -= entry.size
			delete(c.items, id)
		}
	}
	if c.used < 0 {
		c.used = 0
	}
}

func (c *publicEphemeralBlobCache) enforceBudgetLocked() {
	for c.maxBytes > 0 && c.used > c.maxBytes && len(c.items) > 0 {
		oldestID := ""
		oldestAt := time.Time{}
		for id, entry := range c.items {
			if oldestID == "" || entry.lastAccess.Before(oldestAt) {
				oldestID = id
				oldestAt = entry.lastAccess
			}
		}
		if oldestID == "" {
			break
		}
		evicted := c.items[oldestID]
		c.used -= evicted.size
		delete(c.items, oldestID)
	}
	if c.used < 0 {
		c.used = 0
	}
}
