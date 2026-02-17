package identity

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/securestore"
)

type StateStore struct {
	path   string
	secret string
}

func NewStateStore() *StateStore {
	return &StateStore{}
}

func (s *StateStore) Configure(path, secret string) {
	s.path = strings.TrimSpace(path)
	s.secret = strings.TrimSpace(secret)
}

func (s *StateStore) Bootstrap(identityManager contracts.IdentityDomain) error {
	if strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.secret) == "" {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s.Persist(identityManager)
		}
		return err
	}
	plaintext, err := securestore.Decrypt(s.secret, raw)
	if err != nil {
		return err
	}
	var state persistedIdentityState
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return err
	}
	if state.Version != 1 || len(state.SigningPrivateKey) == 0 {
		return errors.New("identity persistence payload is invalid")
	}
	return identityManager.RestoreIdentityPrivateKey(state.SigningPrivateKey)
}

func (s *StateStore) Persist(identityManager contracts.IdentityDomain) error {
	if strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.secret) == "" {
		return nil
	}
	_, privateKey := identityManager.SnapshotIdentityKeys()
	state := persistedIdentityState{Version: 1, SigningPrivateKey: privateKey}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	encrypted, err := securestore.Encrypt(s.secret, payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, encrypted, 0o600)
}

type persistedIdentityState struct {
	Version           int    `json:"version"`
	SigningPrivateKey []byte `json:"signing_private_key"`
}
