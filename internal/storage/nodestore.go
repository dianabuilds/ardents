package storage

import (
	"errors"
	"sync"
)

var (
	ErrNotFound = errors.New("ERR_NODE_NOT_FOUND")
	ErrTooLarge = errors.New("ERR_NODE_TOO_LARGE")
)

type NodeStore struct {
	mu       sync.RWMutex
	maxBytes int
	nodes    map[string][]byte
}

func NewNodeStore(maxBytes int) *NodeStore {
	if maxBytes <= 0 {
		maxBytes = 1_048_576
	}
	return &NodeStore{
		maxBytes: maxBytes,
		nodes:    make(map[string][]byte),
	}
}

func (s *NodeStore) Put(nodeID string, bytes []byte) error {
	if nodeID == "" || bytes == nil {
		return ErrNotFound
	}
	if len(bytes) > s.maxBytes {
		return ErrTooLarge
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cpy := make([]byte, len(bytes))
	copy(cpy, bytes)
	s.nodes[nodeID] = cpy
	return nil
}

func (s *NodeStore) Get(nodeID string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.nodes[nodeID]
	if !ok {
		return nil, ErrNotFound
	}
	cpy := make([]byte, len(b))
	copy(cpy, b)
	return cpy, nil
}
