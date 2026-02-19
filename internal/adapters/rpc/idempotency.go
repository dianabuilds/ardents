package rpc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const (
	rpcIdempotencyHeader     = "X-AIM-Idempotency-Key"
	rpcIdempotencyTTL        = 10 * time.Minute
	rpcIdempotencyMaxEntries = 1024
)

type rpcIdempotencyEntry struct {
	requestHash string
	response    rpcResponse
	createdAt   time.Time
}

type rpcIdempotencyCache struct {
	entries map[string]rpcIdempotencyEntry
}

func newRPCIdempotencyCache() *rpcIdempotencyCache {
	return &rpcIdempotencyCache{
		entries: make(map[string]rpcIdempotencyEntry),
	}
}

func (c *rpcIdempotencyCache) get(cacheKey, requestHash string, now time.Time) (rpcResponse, bool, bool) {
	if c == nil {
		return rpcResponse{}, false, false
	}
	c.prune(now)
	entry, ok := c.entries[cacheKey]
	if !ok {
		return rpcResponse{}, false, false
	}
	if entry.requestHash != requestHash {
		return rpcResponse{}, false, true
	}
	return entry.response, true, false
}

func (c *rpcIdempotencyCache) set(cacheKey, requestHash string, resp rpcResponse, now time.Time) {
	if c == nil {
		return
	}
	c.prune(now)
	c.entries[cacheKey] = rpcIdempotencyEntry{
		requestHash: requestHash,
		response:    resp,
		createdAt:   now,
	}
	if len(c.entries) <= rpcIdempotencyMaxEntries {
		return
	}
	// Bounded memory: drop oldest entry when over limit.
	var oldestKey string
	var oldestAt time.Time
	first := true
	for key, entry := range c.entries {
		if first || entry.createdAt.Before(oldestAt) {
			oldestKey = key
			oldestAt = entry.createdAt
			first = false
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func (c *rpcIdempotencyCache) prune(now time.Time) {
	for key, entry := range c.entries {
		if now.Sub(entry.createdAt) > rpcIdempotencyTTL {
			delete(c.entries, key)
		}
	}
}

func rpcIdempotencyKey(raw string, authToken string) string {
	key := strings.TrimSpace(raw)
	if key == "" {
		return ""
	}
	return authToken + "|" + key
}

func rpcRequestHash(req rpcRequest) string {
	payload := struct {
		Method     string          `json:"method"`
		Params     json.RawMessage `json:"params"`
		APIVersion *int            `json:"api_version,omitempty"`
	}{
		Method:     req.Method,
		Params:     req.Params,
		APIVersion: req.APIVersion,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(req.Method + "|" + string(req.Params))
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
