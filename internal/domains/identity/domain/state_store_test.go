package domain

import (
	"path/filepath"
	"testing"
)

func TestStateStorePersistAndBootstrapRestoresSeedEnvelope(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "identity.state")
	secret := "test-secret"

	manager, err := NewManager()
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

	reloaded, err := NewManager()
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

func TestStateStorePersistAndBootstrapRestoresRuntimeState(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "identity.state")
	secret := "test-secret"
	contactID := "aim1contact12345"

	manager, err := NewManager()
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, _, err := manager.CreateIdentity("pass-1"); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if err := manager.AddContactByIdentityID(contactID, "Alice"); err != nil {
		t.Fatalf("add contact: %v", err)
	}
	addedDevice, err := manager.AddDevice("laptop")
	if err != nil {
		t.Fatalf("add device: %v", err)
	}

	store := NewStateStore()
	store.Configure(storePath, secret)
	if err := store.Persist(manager); err != nil {
		t.Fatalf("persist: %v", err)
	}

	reloaded, err := NewManager()
	if err != nil {
		t.Fatalf("new reloaded manager: %v", err)
	}
	reloadStore := NewStateStore()
	reloadStore.Configure(storePath, secret)
	if err := reloadStore.Bootstrap(reloaded); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	if !reloaded.HasContact(contactID) {
		t.Fatalf("expected restored contact %q", contactID)
	}
	devices := reloaded.ListDevices()
	if len(devices) < 2 {
		t.Fatalf("expected restored devices, got %d", len(devices))
	}
	found := false
	for _, d := range devices {
		if d.ID == addedDevice.ID && d.Name == "laptop" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected restored added device id=%q", addedDevice.ID)
	}
}
