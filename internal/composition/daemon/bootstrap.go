package daemon

import "strings"

const DefaultDataDir = "go-backend/data"

func ResolveStorage(dataDir string) (resolvedDir, secret string, bundle StorageBundle, err error) {
	resolvedDir = strings.TrimSpace(dataDir)
	if resolvedDir == "" {
		resolvedDir = DefaultDataDir
	}

	secret, err = StoragePassphrase(resolvedDir)
	if err != nil {
		return "", "", StorageBundle{}, err
	}

	bundle, err = BuildStorageBundle(resolvedDir, secret)
	if err == nil {
		return resolvedDir, secret, bundle, nil
	}
	if !ShouldRetryWithLegacySecret(secret, err) {
		return "", "", StorageBundle{}, err
	}

	secret = LegacyStoragePassphrase
	if werr := WriteStorageKey(resolvedDir, secret); werr != nil {
		return "", "", StorageBundle{}, werr
	}
	bundle, err = BuildStorageBundle(resolvedDir, secret)
	if err != nil {
		return "", "", StorageBundle{}, err
	}
	return resolvedDir, secret, bundle, nil
}
