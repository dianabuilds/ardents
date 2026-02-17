package privacy

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"

	"aim-chat/go-backend/internal/securestore"
)

type BlocklistStore struct {
	path   string
	secret string
}

func NewBlocklistStore() *BlocklistStore {
	return &BlocklistStore{}
}

func (s *BlocklistStore) Configure(path, secret string) {
	s.path, s.secret = normalizeStoreConfig(path, secret)
}

func (s *BlocklistStore) Bootstrap() (Blocklist, error) {
	if strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.secret) == "" {
		return NewBlocklist(nil)
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			list, err := NewBlocklist(nil)
			if err != nil {
				return Blocklist{}, err
			}
			if err := s.Persist(list); err != nil {
				return Blocklist{}, err
			}
			return list, nil
		}
		return Blocklist{}, err
	}
	plaintext, err := securestore.Decrypt(s.secret, raw)
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

func (s *BlocklistStore) Persist(list Blocklist) error {
	if strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.secret) == "" {
		return nil
	}
	state := persistedBlocklistState{
		Version: 1,
		Blocked: list.List(),
	}
	return persistEncryptedJSON(s.path, s.secret, state)
}

type persistedBlocklistState struct {
	Version int      `json:"version"`
	Blocked []string `json:"blocked"`
}
