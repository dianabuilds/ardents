package net

import (
	"sync"
	"time"
)

type BanList struct {
	mu    sync.Mutex
	bans  map[string]time.Time
	nowFn func() time.Time
}

func NewBanList() *BanList {
	return &BanList{
		bans:  make(map[string]time.Time),
		nowFn: time.Now,
	}
}

func (b *BanList) Ban(peerID string, window time.Duration) {
	if peerID == "" || window <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bans[peerID] = b.nowFn().Add(window)
}

func (b *BanList) IsBanned(peerID string) bool {
	if peerID == "" {
		return false
	}
	now := b.nowFn()
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.bans[peerID]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(b.bans, peerID)
		return false
	}
	return true
}
