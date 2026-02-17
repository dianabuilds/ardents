package privacy

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"

	"aim-chat/go-backend/internal/securestore"
)

type SettingsStore struct {
	path   string
	secret string
}

func NewSettingsStore() *SettingsStore {
	return &SettingsStore{}
}

func (s *SettingsStore) Configure(path, secret string) {
	s.path, s.secret = normalizeStoreConfig(path, secret)
}

func (s *SettingsStore) Bootstrap() (PrivacySettings, error) {
	if strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.secret) == "" {
		return DefaultPrivacySettings(), nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			settings := DefaultPrivacySettings()
			if err := s.Persist(settings); err != nil {
				return PrivacySettings{}, err
			}
			return settings, nil
		}
		return PrivacySettings{}, err
	}
	plaintext, err := securestore.Decrypt(s.secret, raw)
	if err != nil {
		return PrivacySettings{}, err
	}

	var state persistedPrivacySettingsState
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return PrivacySettings{}, err
	}
	if state.Version != 1 {
		return PrivacySettings{}, errors.New("privacy settings persistence payload is invalid")
	}
	return NormalizePrivacySettings(state.Settings), nil
}

func (s *SettingsStore) Persist(settings PrivacySettings) error {
	if strings.TrimSpace(s.path) == "" || strings.TrimSpace(s.secret) == "" {
		return nil
	}
	state := persistedPrivacySettingsState{
		Version:  1,
		Settings: NormalizePrivacySettings(settings),
	}
	return persistEncryptedJSON(s.path, s.secret, state)
}

type persistedPrivacySettingsState struct {
	Version  int             `json:"version"`
	Settings PrivacySettings `json:"settings"`
}
