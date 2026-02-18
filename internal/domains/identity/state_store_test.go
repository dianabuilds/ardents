package identity

import (
	"path/filepath"
	"testing"

	identitycore "aim-chat/go-backend/internal/identity"
)

func TestStateStorePersistAndBootstrapRestoresSeedEnvelope(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "identity.state")
	secret := "test-secret"

	manager, err := identitycore.NewManager()
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	_, mnemonic, err := manager.CreateIdentity("pass-1")
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}

	store := NewStateStore()
	store.Configure(storePath, secret)
	if err := store.Persist(manager); err != nil {
		t.Fatalf("persist: %v", err)
	}

	reloaded, err := identitycore.NewManager()
	if err != nil {
		t.Fatalf("new reloaded manager: %v", err)
	}
	reloadStore := NewStateStore()
	reloadStore.Configure(storePath, secret)
	if err := reloadStore.Bootstrap(reloaded); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	exported, err := reloaded.ExportSeed("pass-1")
	if err != nil {
		t.Fatalf("export after bootstrap: %v", err)
	}
	if exported != mnemonic {
		t.Fatalf("mnemonic mismatch after bootstrap")
	}

	if err := reloaded.ChangePassword("pass-1", "pass-2"); err != nil {
		t.Fatalf("change password after bootstrap: %v", err)
	}
	if _, err := reloaded.ExportSeed("pass-2"); err != nil {
		t.Fatalf("export with new password after bootstrap: %v", err)
	}
}
