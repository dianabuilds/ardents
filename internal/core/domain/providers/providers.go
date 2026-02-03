package providers

import (
	"sort"
	"sync"
)

type ProviderRecord struct {
	V              uint64 `cbor:"v"`
	NodeID         string `cbor:"node_id"`
	ProviderPeerID string `cbor:"provider_peer_id"`
	TSMs           int64  `cbor:"ts_ms"`
	TTLMs          int64  `cbor:"ttl_ms"`
}

type Registry struct {
	mu        sync.Mutex
	records   map[string][]ProviderRecord
	successes map[string]bool
}

func NewRegistry() *Registry {
	return &Registry{
		records:   make(map[string][]ProviderRecord),
		successes: make(map[string]bool),
	}
}

func (r *Registry) Add(rec ProviderRecord, nowMs int64) {
	if rec.NodeID == "" || rec.ProviderPeerID == "" || rec.TTLMs <= 0 {
		return
	}
	if rec.TSMs == 0 {
		rec.TSMs = nowMs
	}
	if nowMs > 0 && rec.TSMs+rec.TTLMs < nowMs {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.records[rec.NodeID]
	list = append(list, rec)
	r.records[rec.NodeID] = list
}

func (r *Registry) MarkSuccess(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.successes[peerID] = true
}

func (r *Registry) Select(nodeID string, trusted map[string]bool, nowMs int64) []ProviderRecord {
	r.mu.Lock()
	src := r.records[nodeID]
	list := make([]ProviderRecord, 0, len(src))
	for _, rec := range src {
		if nowMs > 0 && rec.TSMs+rec.TTLMs < nowMs {
			continue
		}
		list = append(list, rec)
	}
	r.records[nodeID] = list
	r.mu.Unlock()
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		at := trusted[a.ProviderPeerID]
		bt := trusted[b.ProviderPeerID]
		if at != bt {
			return at && !bt
		}
		as := r.successes[a.ProviderPeerID]
		bs := r.successes[b.ProviderPeerID]
		if as != bs {
			return as && !bs
		}
		if a.TSMs != b.TSMs {
			return a.TSMs > b.TSMs
		}
		return a.ProviderPeerID < b.ProviderPeerID
	})
	return list
}
