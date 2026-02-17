package privacy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/internal/testutil/fsperm"
)

func TestSettingsStoreBootstrapDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "privacy.enc")
	store := NewSettingsStore()
	store.Configure(path, "test-secret")

	got, err := store.Bootstrap()
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	if got.MessagePrivacyMode != DefaultMessagePrivacyMode {
		t.Fatalf("unexpected default mode: got=%q want=%q", got.MessagePrivacyMode, DefaultMessagePrivacyMode)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected privacy settings file to be created, err=%v", err)
	}
}

func TestSettingsStorePersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "privacy.enc")
	store := NewSettingsStore()
	store.Configure(path, "test-secret")

	settings := PrivacySettings{MessagePrivacyMode: MessagePrivacyEveryone}
	if err := store.Persist(settings); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	reload := NewSettingsStore()
	reload.Configure(path, "test-secret")
	got, err := reload.Bootstrap()
	if err != nil {
		t.Fatalf("reload bootstrap failed: %v", err)
	}
	if got.MessagePrivacyMode != MessagePrivacyEveryone {
		t.Fatalf("unexpected reloaded mode: got=%q want=%q", got.MessagePrivacyMode, MessagePrivacyEveryone)
	}
}

func TestSettingsStoreNormalizeOnBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "privacy.enc")
	store := NewSettingsStore()
	store.Configure(path, "test-secret")
	if err := store.Persist(PrivacySettings{MessagePrivacyMode: "invalid"}); err != nil {
		t.Fatalf("persist invalid failed: %v", err)
	}

	got, err := store.Bootstrap()
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	if got.MessagePrivacyMode != DefaultMessagePrivacyMode {
		t.Fatalf("unexpected normalized mode: got=%q want=%q", got.MessagePrivacyMode, DefaultMessagePrivacyMode)
	}
}

func TestSettingsStoreBootstrapIOError(t *testing.T) {
	dir := t.TempDir()
	store := NewSettingsStore()
	store.Configure(dir, "test-secret")

	_, err := store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap io error")
	}
}

func TestSettingsStorePersistCreatesPrivateDir(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "secure", "privacy.enc")
	store := NewSettingsStore()
	store.Configure(path, "test-secret")

	if err := store.Persist(DefaultPrivacySettings()); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	fsperm.AssertPrivateDirPerm(t, filepath.Dir(path))
}

func TestBlocklistStoreBootstrapDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocklist.enc")
	store := NewBlocklistStore()
	store.Configure(path, "test-secret")

	list, err := store.Bootstrap()
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	if len(list.List()) != 0 {
		t.Fatalf("expected empty blocklist, got %d entries", len(list.List()))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected blocklist file to be created, err=%v", err)
	}
}

func TestBlocklistStorePersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocklist.enc")
	store := NewBlocklistStore()
	store.Configure(path, "test-secret")

	list, err := NewBlocklist([]string{"aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF"})
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	if err := store.Persist(list); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	reload := NewBlocklistStore()
	reload.Configure(path, "test-secret")
	got, err := reload.Bootstrap()
	if err != nil {
		t.Fatalf("reload bootstrap failed: %v", err)
	}
	if !got.Contains("aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF") {
		t.Fatal("reloaded blocklist must contain saved id")
	}
}

func TestBlocklistStoreRejectsInvalidPersistedIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocklist.enc")
	store := NewBlocklistStore()
	store.Configure(path, "test-secret")

	payload, err := json.Marshal(persistedBlocklistState{
		Version: 1,
		Blocked: []string{"invalid"},
	})
	if err != nil {
		t.Fatalf("marshal invalid fixture failed: %v", err)
	}
	encrypted, err := securestore.Encrypt("test-secret", payload)
	if err != nil {
		t.Fatalf("encrypt invalid fixture failed: %v", err)
	}
	if err := os.WriteFile(path, encrypted, 0o600); err != nil {
		t.Fatalf("write invalid fixture failed: %v", err)
	}
	_, err = store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap error for invalid persisted ids")
	}
}

func TestBlocklistStoreBootstrapIOError(t *testing.T) {
	dir := t.TempDir()
	store := NewBlocklistStore()
	store.Configure(dir, "test-secret")

	_, err := store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap io error")
	}
}

func TestBlocklistStorePersistCreatesPrivateDir(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "secure", "blocklist.enc")
	store := NewBlocklistStore()
	store.Configure(path, "test-secret")

	list, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	if err := store.Persist(list); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	fsperm.AssertPrivateDirPerm(t, filepath.Dir(path))
}

func TestBlocklistStorePersistRejectsInvalidValue(t *testing.T) {
	store := NewBlocklistStore()
	store.Configure(filepath.Join(t.TempDir(), "b.enc"), "test-secret")
	_, err := NewBlocklist([]string{"invalid"})
	if !errors.Is(err, ErrInvalidIdentityID) {
		t.Fatalf("expected ErrInvalidIdentityID, got %v", err)
	}
}
