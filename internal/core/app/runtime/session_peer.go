package runtime

import (
	"sync"
	"time"
)

type sessionPeerEntry struct {
	peerID string
	seenAt time.Time
}

type sessionPeerStore struct {
	mu   sync.Mutex
	ttl  time.Duration
	byID map[string]sessionPeerEntry
}

func newSessionPeerStore(ttl time.Duration) *sessionPeerStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &sessionPeerStore{
		ttl:  ttl,
		byID: make(map[string]sessionPeerEntry),
	}
}

func (s *sessionPeerStore) Remember(nodeID string, peerID string) {
	if s == nil || nodeID == "" || peerID == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	s.byID[nodeID] = sessionPeerEntry{peerID: peerID, seenAt: now}
}

func (s *sessionPeerStore) Lookup(nodeID string) (string, bool) {
	if s == nil || nodeID == "" {
		return "", false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	entry, ok := s.byID[nodeID]
	if !ok {
		return "", false
	}
	return entry.peerID, true
}

func (s *sessionPeerStore) gcLocked(now time.Time) {
	exp := now.Add(-s.ttl)
	for k, v := range s.byID {
		if v.seenAt.Before(exp) {
			delete(s.byID, k)
		}
	}
}
