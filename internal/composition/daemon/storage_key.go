package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	storagePassphraseEnv     = "AIM_STORAGE_PASSPHRASE"
	legacyMigrationSecretEnv = "AIM_LEGACY_STORAGE_PASSPHRASE"
	storageKeyWrappedEnv     = "AIM_STORAGE_KEY_WRAPPED"
)

var ErrLegacyStorageSecretRequired = errors.New("legacy storage secret is required")
var ErrInsecureStorageKeyMode = errors.New("insecure storage key mode is forbidden in production")

func StoragePassphrase(dataDir string) (string, error) {
	if secret := strings.TrimSpace(os.Getenv(storagePassphraseEnv)); secret != "" {
		return secret, nil
	}
	keyPath := filepath.Join(dataDir, "storage.key")
	existing, err := os.ReadFile(keyPath)
	if err == nil {
		if secret := strings.TrimSpace(string(existing)); secret != "" {
			if policyErr := enforceStorageKeyPolicy("file"); policyErr != nil {
				return "", policyErr
			}
			return secret, nil
		}
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if policyErr := enforceStorageKeyPolicy("auto-generate"); policyErr != nil {
		return "", policyErr
	}
	if hasLegacyPersistentData(dataDir) {
		return "", fmt.Errorf(
			"%w: set %s to current secret or %s for explicit migration",
			ErrLegacyStorageSecretRequired,
			storagePassphraseEnv,
			legacyMigrationSecretEnv,
		)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := base64.RawStdEncoding.EncodeToString(buf)
	if err := WriteStorageKey(dataDir, secret); err != nil {
		return "", err
	}
	return secret, nil
}

func WriteStorageKey(dataDir, secret string) error {
	if policyErr := enforceStorageKeyPolicy("write-file"); policyErr != nil {
		return policyErr
	}
	keyPath := filepath.Join(dataDir, "storage.key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(keyPath, []byte(secret), 0o600)
}

func LegacyMigrationSecret() string {
	return strings.TrimSpace(os.Getenv(legacyMigrationSecretEnv))
}

func hasLegacyPersistentData(dataDir string) bool {
	paths := []string{
		filepath.Join(dataDir, "messages.json"),
		filepath.Join(dataDir, "sessions.json"),
		filepath.Join(dataDir, "identity.enc"),
		filepath.Join(dataDir, "privacy.enc"),
		filepath.Join(dataDir, "blocklist.enc"),
		filepath.Join(dataDir, "attachments", "index.json"),
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() && info.Size() > 0 {
			return true
		}
	}
	return false
}

func enforceStorageKeyPolicy(source string) error {
	if !isProductionEnv() {
		return nil
	}
	if source == "auto-generate" {
		return fmt.Errorf(
			"%w: production requires %s or OS-keystore-wrapped key flow; raw storage.key generation is disabled",
			ErrInsecureStorageKeyMode,
			storagePassphraseEnv,
		)
	}
	wrapped, _ := parseBoolEnv(storageKeyWrappedEnv)
	if wrapped {
		return nil
	}
	return fmt.Errorf(
		"%w: raw storage.key is forbidden in production; set %s or enable wrapped key flow (%s=true)",
		ErrInsecureStorageKeyMode,
		storagePassphraseEnv,
		storageKeyWrappedEnv,
	)
}

func isProductionEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AIM_ENV"))) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func parseBoolEnv(name string) (bool, bool) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}
