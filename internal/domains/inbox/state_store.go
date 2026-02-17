package inbox

import (
	"encoding/json"
	"errors"
	"io/fs"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/pkg/models"
)

type RequestStore struct {
	path   string
	secret string
}

func NewRequestStore() *RequestStore {
	return &RequestStore{}
}

func (s *RequestStore) Configure(path, secret string) {
	s.path, s.secret = securestore.NormalizeStorageConfig(path, secret)
}

func (s *RequestStore) Bootstrap() (map[string][]models.Message, error) {
	if !securestore.IsStorageConfigured(s.path, s.secret) {
		return map[string][]models.Message{}, nil
	}
	plaintext, err := securestore.ReadDecryptedFile(s.path, s.secret)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			inbox := map[string][]models.Message{}
			if err := s.Persist(inbox); err != nil {
				return nil, err
			}
			return inbox, nil
		}
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
	return cloneMessageRequestInbox(state.Inbox), nil
}

func (s *RequestStore) Persist(inbox map[string][]models.Message) error {
	if !securestore.IsStorageConfigured(s.path, s.secret) {
		return nil
	}
	state := persistedMessageRequestState{
		Version: 1,
		Inbox:   cloneMessageRequestInbox(inbox),
	}
	return securestore.WriteEncryptedJSON(s.path, s.secret, state)
}

type persistedMessageRequestState struct {
	Version int                         `json:"version"`
	Inbox   map[string][]models.Message `json:"inbox"`
}

func cloneMessageRequestInbox(inbox map[string][]models.Message) map[string][]models.Message {
	if inbox == nil {
		return map[string][]models.Message{}
	}
	out := make(map[string][]models.Message, len(inbox))
	for senderID, messages := range inbox {
		cloned := make([]models.Message, len(messages))
		for i := range messages {
			cloned[i] = messages[i]
			cloned[i].Content = append([]byte(nil), messages[i].Content...)
		}
		out[senderID] = cloned
	}
	return out
}
