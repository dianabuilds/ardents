package identity

import (
	"errors"
	"testing"
	"time"
)

func TestSeedLifecycleCreateExportImport(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}

	createdIdentity, mnemonic, err := mgr.CreateIdentity("pass-1")
	if err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	if !mgr.ValidateMnemonic(mnemonic) {
		t.Fatal("created mnemonic must be valid")
	}

	exported, err := mgr.ExportSeed("pass-1")
	if err != nil {
		t.Fatalf("export seed failed: %v", err)
	}
	if exported != mnemonic {
		t.Fatal("exported mnemonic should match created mnemonic")
	}

	importedIdentity, err := mgr.ImportIdentity(mnemonic, "pass-2")
	if err != nil {
		t.Fatalf("import identity failed: %v", err)
	}
	if createdIdentity.ID != importedIdentity.ID {
		t.Fatal("importing same mnemonic should reproduce same identity id")
	}
}

func TestSeedLifecycleInvalidInputs(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	if _, err := mgr.ExportSeed("p"); err == nil {
		t.Fatal("expected error exporting without stored seed")
	}
	if _, _, err := mgr.CreateIdentity(""); err == nil {
		t.Fatal("expected error for empty password")
	}
	if _, err := mgr.ImportIdentity("not a mnemonic", "p"); err == nil {
		t.Fatal("expected error for invalid mnemonic")
	}
}

func TestSeedLifecycleChangePassword(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	_, mnemonic, err := mgr.CreateIdentity("old-pass")
	if err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	if err := mgr.ChangePassword("old-pass", "new-pass"); err != nil {
		t.Fatalf("change password failed: %v", err)
	}
	exported, err := mgr.ExportSeed("new-pass")
	if err != nil {
		t.Fatalf("new password export failed: %v", err)
	}
	if exported != mnemonic {
		t.Fatal("mnemonic should stay unchanged after password change")
	}
	if _, err := mgr.ExportSeed("old-pass"); err == nil {
		t.Fatal("expected old password to fail after password change")
	}
}

func TestSeedLifecyclePasswordLockout(t *testing.T) {
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	sm := newSeedManagerWithClock(clock)

	mnemonic, _, err := sm.Create("good-pass")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !sm.ValidateMnemonic(mnemonic) {
		t.Fatal("mnemonic should be valid")
	}

	if _, err := sm.Export("wrong-pass"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
	if _, err := sm.Export("wrong-pass"); !errors.Is(err, ErrPasswordLocked) {
		t.Fatalf("expected ErrPasswordLocked, got %v", err)
	}

	now = now.Add(2 * time.Second)
	if _, err := sm.Export("good-pass"); err != nil {
		t.Fatalf("expected unlock after backoff, got %v", err)
	}
}
