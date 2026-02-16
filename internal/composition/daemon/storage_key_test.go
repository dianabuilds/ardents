package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"aim-chat/go-backend/internal/securestore"
)

func TestStoragePassphraseLegacyDataRequiresExplicitSecret(t *testing.T) {
	t.Setenv(storagePassphraseEnv, "")
	t.Setenv(legacyMigrationSecretEnv, "")

	dataDir := t.TempDir()
	msgPath := filepath.Join(dataDir, "messages.json")
	if err := os.WriteFile(msgPath, []byte("legacy"), 0o600); err != nil {
		t.Fatalf("write legacy marker failed: %v", err)
	}

	_, err := StoragePassphrase(dataDir)
	if !errors.Is(err, ErrLegacyStorageSecretRequired) {
		t.Fatalf("expected ErrLegacyStorageSecretRequired, got: %v", err)
	}
}

func TestResolveStorageUsesExplicitLegacySecretWhenLegacyDataDetected(t *testing.T) {
	t.Setenv(storagePassphraseEnv, "")
	legacySecret := "explicit-legacy-secret-for-migration"
	t.Setenv(legacyMigrationSecretEnv, legacySecret)

	dataDir := t.TempDir()
	enc, err := securestore.Encrypt(legacySecret, []byte(`{"messages":{},"pending":{}}`))
	if err != nil {
		t.Fatalf("encrypt fixture failed: %v", err)
	}
	msgPath := filepath.Join(dataDir, "messages.json")
	if err := os.WriteFile(msgPath, enc, 0o600); err != nil {
		t.Fatalf("write encrypted messages failed: %v", err)
	}

	resolved, secret, _, err := ResolveStorage(dataDir)
	if err != nil {
		t.Fatalf("resolve storage failed: %v", err)
	}
	if resolved != dataDir {
		t.Fatalf("unexpected resolved dir: %s", resolved)
	}
	if secret != legacySecret {
		t.Fatalf("expected explicit legacy secret, got: %s", secret)
	}
	keyBytes, err := os.ReadFile(filepath.Join(dataDir, "storage.key"))
	if err != nil {
		t.Fatalf("read storage key failed: %v", err)
	}
	if string(keyBytes) != legacySecret {
		t.Fatalf("storage key must be updated with explicit legacy secret")
	}
}

func TestResolveStorageRetriesWithExplicitLegacySecretOnAuthFailure(t *testing.T) {
	t.Setenv(storagePassphraseEnv, "")
	legacySecret := "legacy-secret-auth-retry"
	t.Setenv(legacyMigrationSecretEnv, legacySecret)

	dataDir := t.TempDir()
	if err := WriteStorageKey(dataDir, "wrong-secret"); err != nil {
		t.Fatalf("write wrong key failed: %v", err)
	}
	enc, err := securestore.Encrypt(legacySecret, []byte(`{"messages":{},"pending":{}}`))
	if err != nil {
		t.Fatalf("encrypt fixture failed: %v", err)
	}
	msgPath := filepath.Join(dataDir, "messages.json")
	if err := os.WriteFile(msgPath, enc, 0o600); err != nil {
		t.Fatalf("write encrypted messages failed: %v", err)
	}

	_, secret, _, err := ResolveStorage(dataDir)
	if err != nil {
		t.Fatalf("resolve storage must fallback to explicit legacy secret: %v", err)
	}
	if secret != legacySecret {
		t.Fatalf("expected explicit legacy secret, got: %s", secret)
	}
	keyBytes, err := os.ReadFile(filepath.Join(dataDir, "storage.key"))
	if err != nil {
		t.Fatalf("read storage key failed: %v", err)
	}
	if string(keyBytes) != legacySecret {
		t.Fatalf("storage key must be replaced with explicit legacy secret")
	}
}
