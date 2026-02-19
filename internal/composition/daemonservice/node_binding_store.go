package daemonservice

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/pkg/models"
)

type nodeBindingStore struct {
	mu       sync.RWMutex
	path     string
	secret   string
	bindings map[string]models.NodeBindingRecord
}

func newNodeBindingStore() *nodeBindingStore {
	return &nodeBindingStore{
		bindings: map[string]models.NodeBindingRecord{},
	}
}

func (s *nodeBindingStore) Configure(path, secret string) {
	s.path, s.secret = securestore.NormalizeStorageConfig(path, secret)
}

func (s *nodeBindingStore) Bootstrap() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !securestore.IsStorageConfigured(s.path, s.secret) {
		s.bindings = map[string]models.NodeBindingRecord{}
		return nil
	}
	plaintext, err := securestore.ReadDecryptedFile(s.path, s.secret)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			s.bindings = map[string]models.NodeBindingRecord{}
			return s.persistLocked()
		}
		return err
	}
	var payload persistedNodeBindings
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return err
	}
	if payload.Version != 1 {
		return errors.New("node binding persistence payload is invalid")
	}
	if payload.Bindings == nil {
		payload.Bindings = map[string]models.NodeBindingRecord{}
	}
	s.bindings = payload.Bindings
	return nil
}

func (s *nodeBindingStore) Get(identityID string) (models.NodeBindingRecord, bool) {
	identityID = strings.TrimSpace(identityID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.bindings[identityID]
	return record, ok
}

func (s *nodeBindingStore) Upsert(record models.NodeBindingRecord) error {
	if strings.TrimSpace(record.IdentityID) == "" {
		return errors.New("identity id is required")
	}
	if strings.TrimSpace(record.NodeID) == "" {
		return errors.New("node id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[record.IdentityID] = record
	return s.persistLocked()
}

func (s *nodeBindingStore) Delete(identityID string) error {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return errors.New("identity id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bindings, identityID)
	return s.persistLocked()
}

func (s *nodeBindingStore) Wipe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings = map[string]models.NodeBindingRecord{}
	if s.path == "" {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *nodeBindingStore) persistLocked() error {
	if !securestore.IsStorageConfigured(s.path, s.secret) {
		return nil
	}
	payload := persistedNodeBindings{
		Version:  1,
		Bindings: s.bindings,
	}
	return securestore.WriteEncryptedJSON(s.path, s.secret, payload)
}

type pendingNodeBindingLink struct {
	IdentityID string
	Code       string
	Challenge  string
	ExpiresAt  time.Time
}

type persistedNodeBindings struct {
	Version  int                                 `json:"version"`
	Bindings map[string]models.NodeBindingRecord `json:"bindings"`
}
