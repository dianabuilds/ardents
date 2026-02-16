package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivacySettingsStateStoreBootstrapDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "privacy.enc")
	store := &PrivacySettingsStateStore{}
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

func TestPrivacySettingsStateStorePersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "privacy.enc")
	store := &PrivacySettingsStateStore{}
	store.Configure(path, "test-secret")

	settings := PrivacySettings{MessagePrivacyMode: MessagePrivacyEveryone}
	if err := store.Persist(settings); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	reload := &PrivacySettingsStateStore{}
	reload.Configure(path, "test-secret")
	got, err := reload.Bootstrap()
	if err != nil {
		t.Fatalf("reload bootstrap failed: %v", err)
	}
	if got.MessagePrivacyMode != MessagePrivacyEveryone {
		t.Fatalf("unexpected reloaded mode: got=%q want=%q", got.MessagePrivacyMode, MessagePrivacyEveryone)
	}
}

func TestPrivacySettingsStateStoreNormalizeOnBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "privacy.enc")
	store := &PrivacySettingsStateStore{}
	store.Configure(path, "test-secret")
	if err := store.Persist(PrivacySettings{MessagePrivacyMode: MessagePrivacyMode("invalid")}); err != nil {
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

func TestPrivacySettingsStateStoreBootstrapIOError(t *testing.T) {
	dir := t.TempDir()
	store := &PrivacySettingsStateStore{}
	store.Configure(dir, "test-secret") // directory path forces os.ReadFile error

	_, err := store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap io error")
	}
}

func TestPrivacySettingsStateStorePersistCreatesPrivateDir(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "secure", "privacy.enc")
	store := &PrivacySettingsStateStore{}
	store.Configure(path, "test-secret")

	if err := store.Persist(DefaultPrivacySettings()); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	assertPrivateDirPerm(t, filepath.Dir(path))
}
