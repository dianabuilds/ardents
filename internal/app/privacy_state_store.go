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

type PrivacySettingsStateStore struct {
	path   string
	secret string
}

func (p *PrivacySettingsStateStore) Configure(path, secret string) {
	p.path = strings.TrimSpace(path)
	p.secret = strings.TrimSpace(secret)
}

func (p *PrivacySettingsStateStore) Bootstrap() (PrivacySettings, error) {
	if strings.TrimSpace(p.path) == "" || strings.TrimSpace(p.secret) == "" {
		return DefaultPrivacySettings(), nil
	}
	raw, err := os.ReadFile(p.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			settings := DefaultPrivacySettings()
			if err := p.Persist(settings); err != nil {
				return PrivacySettings{}, err
			}
			return settings, nil
		}
		return PrivacySettings{}, err
	}
	plaintext, err := securestore.Decrypt(p.secret, raw)
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
	settings := NormalizePrivacySettings(state.Settings)
	return settings, nil
}

func (p *PrivacySettingsStateStore) Persist(settings PrivacySettings) error {
	if strings.TrimSpace(p.path) == "" || strings.TrimSpace(p.secret) == "" {
		return nil
	}
	settings = NormalizePrivacySettings(settings)
	state := persistedPrivacySettingsState{
		Version:  1,
		Settings: settings,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	encrypted, err := securestore.Encrypt(p.secret, payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p.path, encrypted, 0o600)
}

type persistedPrivacySettingsState struct {
	Version  int             `json:"version"`
	Settings PrivacySettings `json:"settings"`
}
