package securestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// NormalizeStorageConfig trims persisted path/secret values.
func NormalizeStorageConfig(path, secret string) (string, string) {
	return strings.TrimSpace(path), strings.TrimSpace(secret)
}

// IsStorageConfigured reports whether encrypted persistence is configured.
func IsStorageConfigured(path, secret string) bool {
	return strings.TrimSpace(path) != "" && strings.TrimSpace(secret) != ""
}

// ReadDecryptedFile reads and decrypts file content with the provided secret.
func ReadDecryptedFile(path, secret string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decrypt(secret, raw)
}

// WriteEncryptedJSON marshals, encrypts and writes JSON payload atomically enough for state snapshots.
func WriteEncryptedJSON(path, secret string, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	encrypted, err := Encrypt(secret, payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, encrypted, 0o600)
}
