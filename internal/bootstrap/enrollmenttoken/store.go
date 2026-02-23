package enrollmenttoken

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type InMemoryStore struct {
	mu       sync.Mutex
	redeemed map[string]time.Time
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{redeemed: map[string]time.Time{}}
}

func (s *InMemoryStore) TryRedeem(tokenID string, at time.Time) (bool, error) {
	if tokenID == "" {
		return false, errors.New("token id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.redeemed[tokenID]; exists {
		return false, nil
	}
	s.redeemed[tokenID] = at.UTC()
	return true, nil
}

type FileStore struct {
	mu   sync.Mutex
	path string
	seen map[string]time.Time
}

type filePayload struct {
	Seen map[string]time.Time `json:"seen"`
}

func NewFileStore(path string) *FileStore {
	return &FileStore{
		path: path,
		seen: map[string]time.Time{},
	}
}

func (s *FileStore) Bootstrap() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.path) == "" {
		return errors.New("file store path is required")
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.seen = map[string]time.Time{}
			return nil
		}
		return err
	}
	var payload filePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	if payload.Seen == nil {
		payload.Seen = map[string]time.Time{}
	}
	s.seen = payload.Seen
	return nil
}

func (s *FileStore) TryRedeem(tokenID string, at time.Time) (bool, error) {
	if tokenID == "" {
		return false, errors.New("token id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.seen[tokenID]; exists {
		return false, nil
	}
	s.seen[tokenID] = at.UTC()
	return true, s.persistLocked()
}

func (s *FileStore) persistLocked() error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("file store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(filePayload{Seen: s.seen})
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o600)
}
