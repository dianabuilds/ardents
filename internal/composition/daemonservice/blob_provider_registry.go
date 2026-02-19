package daemonservice

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"aim-chat/go-backend/internal/platform/ratelimiter"
	"aim-chat/go-backend/pkg/models"
)

type blobProviderFetchFn func(blobID, requesterPeerID string) (models.AttachmentMeta, []byte, error)

type blobProviderEntry struct {
	peerID  string
	expires time.Time
	fetchFn blobProviderFetchFn
}

type blobProviderCandidate struct {
	peerID  string
	expires time.Time
	fetchFn blobProviderFetchFn
}

type blobProviderRegistry struct {
	mu       sync.Mutex
	byBlob   map[string]map[string]blobProviderEntry
	announce *ratelimiter.MapLimiter
	fetch    *ratelimiter.MapLimiter
}

func newBlobProviderRegistry() *blobProviderRegistry {
	return &blobProviderRegistry{
		byBlob:   make(map[string]map[string]blobProviderEntry),
		announce: ratelimiter.New(25, 50, 10*time.Minute),
		fetch:    ratelimiter.New(40, 80, 10*time.Minute),
	}
}

func (r *blobProviderRegistry) announceBlob(blobID, peerID string, ttl time.Duration, fetchFn blobProviderFetchFn, now time.Time) error {
	if r == nil {
		return nil
	}
	blobID = strings.TrimSpace(blobID)
	peerID = strings.TrimSpace(peerID)
	if blobID == "" || peerID == "" || fetchFn == nil {
		return errors.New("invalid provider announcement")
	}
	if ttl <= 0 {
		return errors.New("invalid provider ttl")
	}
	if !r.announce.Allow(peerID, now) {
		return errors.New("provider announce rate limit exceeded")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	providers := r.byBlob[blobID]
	if providers == nil {
		providers = make(map[string]blobProviderEntry)
		r.byBlob[blobID] = providers
	}
	providers[peerID] = blobProviderEntry{
		peerID:  peerID,
		expires: now.Add(ttl),
		fetchFn: fetchFn,
	}
	return nil
}

func (r *blobProviderRegistry) listProviders(blobID string, now time.Time) []blobProviderCandidate {
	if r == nil {
		return nil
	}
	blobID = strings.TrimSpace(blobID)
	if blobID == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	providers := r.byBlob[blobID]
	if len(providers) == 0 {
		return nil
	}
	candidates := make([]blobProviderCandidate, 0, len(providers))
	for peerID, provider := range providers {
		if !provider.expires.After(now) {
			delete(providers, peerID)
			continue
		}
		candidates = append(candidates, blobProviderCandidate(provider))
	}
	if len(providers) == 0 {
		delete(r.byBlob, blobID)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].expires.Equal(candidates[j].expires) {
			return candidates[i].peerID < candidates[j].peerID
		}
		return candidates[i].expires.After(candidates[j].expires)
	})
	return candidates
}

func (r *blobProviderRegistry) allowFetch(requesterPeerID, providerPeerID string, now time.Time) bool {
	if r == nil {
		return true
	}
	key := strings.TrimSpace(requesterPeerID) + "->" + strings.TrimSpace(providerPeerID)
	return r.fetch.Allow(key, now)
}

func (r *blobProviderRegistry) removePeer(peerID string) {
	if r == nil {
		return
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for blobID, providers := range r.byBlob {
		delete(providers, peerID)
		if len(providers) == 0 {
			delete(r.byBlob, blobID)
		}
	}
}

func (r *blobProviderRegistry) removeBlobPeer(blobID, peerID string) {
	if r == nil {
		return
	}
	blobID = strings.TrimSpace(blobID)
	peerID = strings.TrimSpace(peerID)
	if blobID == "" || peerID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	providers := r.byBlob[blobID]
	if len(providers) == 0 {
		return
	}
	delete(providers, peerID)
	if len(providers) == 0 {
		delete(r.byBlob, blobID)
	}
}
