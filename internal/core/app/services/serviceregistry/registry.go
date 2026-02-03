package serviceregistry

import (
	"sync"

	"github.com/dianabuilds/ardents/internal/core/app/services/servicedesc"
)

type Descriptor struct {
	NodeID       string
	CreatedAtMs  int64
	Body         servicedesc.DescriptorBody
	SourcePeerID string
}

type Registry struct {
	mu   sync.RWMutex
	byID map[string]Descriptor
}

func New() *Registry {
	return &Registry{byID: make(map[string]Descriptor)}
}

func (r *Registry) UpdateIfNewer(serviceID string, nodeID string, createdAtMs int64, body servicedesc.DescriptorBody, sourcePeerID string) bool {
	if serviceID == "" || nodeID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.byID[serviceID]
	if !ok {
		r.byID[serviceID] = Descriptor{NodeID: nodeID, CreatedAtMs: createdAtMs, Body: body, SourcePeerID: sourcePeerID}
		return true
	}
	if createdAtMs > cur.CreatedAtMs {
		r.byID[serviceID] = Descriptor{NodeID: nodeID, CreatedAtMs: createdAtMs, Body: body, SourcePeerID: sourcePeerID}
		return true
	}
	if createdAtMs == cur.CreatedAtMs && nodeID < cur.NodeID {
		r.byID[serviceID] = Descriptor{NodeID: nodeID, CreatedAtMs: createdAtMs, Body: body, SourcePeerID: sourcePeerID}
		return true
	}
	return false
}

func (r *Registry) Get(serviceID string) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.byID[serviceID]
	return desc, ok
}

func (r *Registry) PurgeByPeer(peerID string) int {
	if peerID == "" {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for id, desc := range r.byID {
		if desc.SourcePeerID == peerID || hasEndpointPeer(desc.Body.Endpoints, peerID) {
			delete(r.byID, id)
			removed++
		}
	}
	return removed
}

func hasEndpointPeer(endpoints []servicedesc.Endpoint, peerID string) bool {
	for _, ep := range endpoints {
		if ep.PeerID == peerID {
			return true
		}
	}
	return false
}
