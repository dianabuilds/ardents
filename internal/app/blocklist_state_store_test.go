package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBlocklistStateStoreBootstrapDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocklist.enc")
	store := &BlocklistStateStore{}
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

func TestBlocklistStateStorePersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocklist.enc")
	store := &BlocklistStateStore{}
	store.Configure(path, "test-secret")

	list, err := NewBlocklist([]string{"aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF"})
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	if err := store.Persist(list); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	reload := &BlocklistStateStore{}
	reload.Configure(path, "test-secret")
	got, err := reload.Bootstrap()
	if err != nil {
		t.Fatalf("reload bootstrap failed: %v", err)
	}
	if !got.Contains("aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF") {
		t.Fatal("reloaded blocklist must contain saved id")
	}
}

func TestBlocklistStateStoreRejectsInvalidPersistedIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocklist.enc")
	store := &BlocklistStateStore{}
	store.Configure(path, "test-secret")

	invalid := Blocklist{entries: map[string]struct{}{"invalid": {}}}
	if err := store.Persist(invalid); err != nil {
		t.Fatalf("persist invalid fixture failed: %v", err)
	}
	_, err := store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap error for invalid persisted ids")
	}
}

func TestBlocklistStateStoreBootstrapIOError(t *testing.T) {
	dir := t.TempDir()
	store := &BlocklistStateStore{}
	store.Configure(dir, "test-secret") // directory path forces os.ReadFile error

	_, err := store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap io error")
	}
}

func TestBlocklistStateStorePersistCreatesPrivateDir(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "secure", "blocklist.enc")
	store := &BlocklistStateStore{}
	store.Configure(path, "test-secret")

	list, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	if err := store.Persist(list); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	assertPrivateDirPerm(t, filepath.Dir(path))
}
