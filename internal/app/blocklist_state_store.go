package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aim-chat/go-backend/internal/securestore"
)

type BlocklistStateStore struct {
	path   string
	secret string
}

func (b *BlocklistStateStore) Configure(path, secret string) {
	b.path = strings.TrimSpace(path)
	b.secret = strings.TrimSpace(secret)
}

func (b *BlocklistStateStore) Bootstrap() (Blocklist, error) {
	if strings.TrimSpace(b.path) == "" || strings.TrimSpace(b.secret) == "" {
		return NewBlocklist(nil)
	}
	raw, err := os.ReadFile(b.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			list, err := NewBlocklist(nil)
			if err != nil {
				return Blocklist{}, err
			}
			if err := b.Persist(list); err != nil {
				return Blocklist{}, err
			}
			return list, nil
		}
		return Blocklist{}, err
	}
	plaintext, err := securestore.Decrypt(b.secret, raw)
	if err != nil {
		return Blocklist{}, err
	}

	var state persistedBlocklistState
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return Blocklist{}, err
	}
	if state.Version != 1 {
		return Blocklist{}, errors.New("blocklist persistence payload is invalid")
	}
	return NewBlocklist(state.Blocked)
}

func (b *BlocklistStateStore) Persist(list Blocklist) error {
	if strings.TrimSpace(b.path) == "" || strings.TrimSpace(b.secret) == "" {
		return nil
	}
	state := persistedBlocklistState{
		Version: 1,
		Blocked: list.List(),
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	encrypted, err := securestore.Encrypt(b.secret, payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(b.path, encrypted, 0o600)
}

type persistedBlocklistState struct {
	Version int      `json:"version"`
	Blocked []string `json:"blocked"`
}
