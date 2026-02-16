package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/pkg/models"
)

type MessageRequestStateStore struct {
	path   string
	secret string
}

func (m *MessageRequestStateStore) Configure(path, secret string) {
	m.path = strings.TrimSpace(path)
	m.secret = strings.TrimSpace(secret)
}

func (m *MessageRequestStateStore) Bootstrap() (map[string][]models.Message, error) {
	if strings.TrimSpace(m.path) == "" || strings.TrimSpace(m.secret) == "" {
		return map[string][]models.Message{}, nil
	}
	raw, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			inbox := map[string][]models.Message{}
			if err := m.Persist(inbox); err != nil {
				return nil, err
			}
			return inbox, nil
		}
		return nil, err
	}
	plaintext, err := securestore.Decrypt(m.secret, raw)
	if err != nil {
		return nil, err
	}

	var state persistedMessageRequestState
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return nil, err
	}
	if state.Version != 1 {
		return nil, errors.New("message request persistence payload is invalid")
	}
	if state.Inbox == nil {
		return map[string][]models.Message{}, nil
	}
	return CloneMessageRequestInbox(state.Inbox), nil
}

func (m *MessageRequestStateStore) Persist(inbox map[string][]models.Message) error {
	if strings.TrimSpace(m.path) == "" || strings.TrimSpace(m.secret) == "" {
		return nil
	}
	state := persistedMessageRequestState{
		Version: 1,
		Inbox:   CloneMessageRequestInbox(inbox),
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	encrypted, err := securestore.Encrypt(m.secret, payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(m.path, encrypted, 0o600)
}

type persistedMessageRequestState struct {
	Version int                         `json:"version"`
	Inbox   map[string][]models.Message `json:"inbox"`
}
