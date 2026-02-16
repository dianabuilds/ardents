package daemon

import (
	"errors"
	"fmt"
	"strings"

	"aim-chat/go-backend/internal/securestore"
)

const DefaultDataDir = "go-backend/data"

func ResolveStorage(dataDir string) (resolvedDir, secret string, bundle StorageBundle, err error) {
	resolvedDir = strings.TrimSpace(dataDir)
	if resolvedDir == "" {
		resolvedDir = DefaultDataDir
	}

	secret, err = StoragePassphrase(resolvedDir)
	if err != nil {
		if !errors.Is(err, ErrLegacyStorageSecretRequired) {
			return "", "", StorageBundle{}, err
		}
		secret = LegacyMigrationSecret()
		if secret == "" {
			return "", "", StorageBundle{}, err
		}
		if werr := WriteStorageKey(resolvedDir, secret); werr != nil {
			return "", "", StorageBundle{}, werr
		}
	}

	bundle, err = BuildStorageBundle(resolvedDir, secret)
	if err == nil {
		return resolvedDir, secret, bundle, nil
	}
	if !errors.Is(err, securestore.ErrAuthFailed) {
		return "", "", StorageBundle{}, err
	}
	legacySecret := LegacyMigrationSecret()
	if legacySecret == "" || legacySecret == secret {
		return "", "", StorageBundle{}, fmt.Errorf(
			"storage authentication failed: set %s to correct secret or %s for explicit migration: %w",
			storagePassphraseEnv,
			legacyMigrationSecretEnv,
			err,
		)
	}
	if werr := WriteStorageKey(resolvedDir, legacySecret); werr != nil {
		return "", "", StorageBundle{}, werr
	}
	bundle, err = BuildStorageBundle(resolvedDir, legacySecret)
	if err != nil {
		return "", "", StorageBundle{}, err
	}
	return resolvedDir, legacySecret, bundle, nil
}
