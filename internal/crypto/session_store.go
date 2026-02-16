package crypto

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"aim-chat/go-backend/internal/securestore"
)

type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]SessionState
}

func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{sessions: make(map[string]SessionState)}
}

func (s *InMemorySessionStore) Save(state SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[state.ContactID] = state
	return nil
}

func (s *InMemorySessionStore) Get(contactID string) (SessionState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.sessions[contactID]
	return state, ok, nil
}

func (s *InMemorySessionStore) All() ([]SessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SessionState, 0, len(s.sessions))
	for _, state := range s.sessions {
		out = append(out, state)
	}
	return out, nil
}

type FileSessionStore struct {
	mu     sync.Mutex
	path   string
	secret string
}

func NewFileSessionStore(path string) *FileSessionStore {
	return &FileSessionStore{path: path}
}

func NewEncryptedFileSessionStore(path, passphrase string) *FileSessionStore {
	return &FileSessionStore{path: path, secret: passphrase}
}

func (s *FileSessionStore) Save(state SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.loadAllLocked()
	if err != nil {
		return err
	}
	all[state.ContactID] = state
	return s.writeAllLocked(all)
}

func (s *FileSessionStore) Get(contactID string) (SessionState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.loadAllLocked()
	if err != nil {
		return SessionState{}, false, err
	}
	state, ok := all[contactID]
	return state, ok, nil
}

func (s *FileSessionStore) All() ([]SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}
	out := make([]SessionState, 0, len(all))
	for _, state := range all {
		out = append(out, state)
	}
	return out, nil
}

func (s *FileSessionStore) loadAllLocked() (map[string]SessionState, error) {
	result := make(map[string]SessionState)
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return result, nil
	}

	decoded := data
	if s.secret != "" {
		plain, err := securestore.Decrypt(s.secret, data)
		if err != nil {
			if errors.Is(err, securestore.ErrLegacyData) {
				decoded = data
			} else {
				return nil, err
			}
		} else {
			decoded = plain
		}
	}

	if err := json.Unmarshal(decoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *FileSessionStore) writeAllLocked(all map[string]SessionState) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(all)
	if err != nil {
		return err
	}
	if s.secret != "" {
		data, err = securestore.Encrypt(s.secret, data)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(s.path, data, 0o600)
}
