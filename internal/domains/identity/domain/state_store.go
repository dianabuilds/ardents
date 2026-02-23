package domain

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
	if err := identityManager.RestoreIdentityPrivateKey(state.SigningPrivateKey); err != nil {
		return err
	}
	if len(state.SeedEnvelope) > 0 {
		if restorer, ok := identityManager.(interface {
			RestoreSeedEnvelopeJSON(raw []byte) error
		}); ok {
			if err := restorer.RestoreSeedEnvelopeJSON(state.SeedEnvelope); err != nil {
				return err
			}
		}
	}
	if len(state.RuntimeState) > 0 {
		if restorer, ok := identityManager.(interface {
			RestoreRuntimeStateJSON(raw []byte) error
		}); ok {
			if err := restorer.RestoreRuntimeStateJSON(state.RuntimeState); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *StateStore) Persist(identityManager contracts.IdentityDomain) error {
	if strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.secret) == "" {
		return nil
	}
	_, privateKey := identityManager.SnapshotIdentityKeys()
	state := persistedIdentityState{Version: 1, SigningPrivateKey: privateKey}
	if snapshotter, ok := identityManager.(interface {
		SnapshotSeedEnvelopeJSON() []byte
	}); ok {
		state.SeedEnvelope = snapshotter.SnapshotSeedEnvelopeJSON()
	}
	if snapshotter, ok := identityManager.(interface {
		SnapshotRuntimeStateJSON() []byte
	}); ok {
		state.RuntimeState = snapshotter.SnapshotRuntimeStateJSON()
	}
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

func (s *StateStore) Wipe() error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *StateStore) StorageDir() string {
	if strings.TrimSpace(s.path) == "" {
		return ""
	}
	return filepath.Dir(s.path)
}

type persistedIdentityState struct {
	Version           int    `json:"version"`
	SigningPrivateKey []byte `json:"signing_private_key"`
	SeedEnvelope      []byte `json:"seed_envelope,omitempty"`
	RuntimeState      []byte `json:"runtime_state,omitempty"`
}
