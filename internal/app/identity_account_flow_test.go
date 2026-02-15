package app

import (
	"bytes"
	"errors"
	"testing"

	"aim-chat/go-backend/pkg/models"
)

type fakeCreateAccountIdentity struct {
	identity models.Identity
	err      error
}

func (f *fakeCreateAccountIdentity) CreateIdentity(_ string) (models.Identity, string, error) {
	if f.err != nil {
		return models.Identity{}, "", f.err
	}
	return f.identity, "mnemonic", nil
}

type fakeLoginIdentity struct {
	identity models.Identity
	err      error
}

func (f *fakeLoginIdentity) GetIdentity() models.Identity {
	return f.identity
}

func (f *fakeLoginIdentity) VerifyPassword(_ string) error {
	return f.err
}

type fakeCreateIdentityAccess struct {
	identity models.Identity
	mnemonic string
	err      error
}

func (f *fakeCreateIdentityAccess) CreateIdentity(_ string) (models.Identity, string, error) {
	if f.err != nil {
		return models.Identity{}, "", f.err
	}
	return f.identity, f.mnemonic, nil
}

type fakeImportIdentityAccess struct {
	identity models.Identity
	err      error
}

func (f *fakeImportIdentityAccess) ImportIdentity(_, _ string) (models.Identity, error) {
	if f.err != nil {
		return models.Identity{}, f.err
	}
	return f.identity, nil
}

func TestCreateAccount(t *testing.T) {
	id := &fakeCreateAccountIdentity{
		identity: models.Identity{ID: "u1", SigningPublicKey: []byte("pk")},
	}
	account, err := CreateAccount("pass", id)
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}
	if account.ID != "u1" || !bytes.Equal(account.IdentityPublicKey, []byte("pk")) {
		t.Fatalf("unexpected account: %#v", account)
	}
}

func TestLoginValidationAndMismatch(t *testing.T) {
	id := &fakeLoginIdentity{identity: models.Identity{ID: "u1"}}

	if err := Login("", "pass", id); err == nil {
		t.Fatal("expected validation error")
	}
	if err := Login("u2", "pass", id); err == nil {
		t.Fatal("expected account mismatch error")
	}
}

func TestCreateIdentityPersist(t *testing.T) {
	id := &fakeCreateIdentityAccess{
		identity: models.Identity{ID: "u1"},
		mnemonic: "mnemonic",
	}
	persistCalls := 0
	persist := func() error {
		persistCalls++
		return nil
	}
	identity, mnemonic, err := CreateIdentity(" pass ", id, persist)
	if err != nil {
		t.Fatalf("CreateIdentity failed: %v", err)
	}
	if identity.ID != "u1" || mnemonic != "mnemonic" {
		t.Fatalf("unexpected values: %#v, %q", identity, mnemonic)
	}
	if persistCalls != 1 {
		t.Fatalf("persist calls = %d, want 1", persistCalls)
	}
}

func TestCreateIdentityPersistError(t *testing.T) {
	id := &fakeCreateIdentityAccess{
		identity: models.Identity{ID: "u1"},
		mnemonic: "mnemonic",
	}
	wantErr := errors.New("persist failed")
	_, _, err := CreateIdentity("pass", id, func() error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected persist error, got %v", err)
	}
}

func TestImportIdentityPersist(t *testing.T) {
	id := &fakeImportIdentityAccess{
		identity: models.Identity{ID: "u1"},
	}
	persistCalls := 0
	identity, err := ImportIdentity(" words ", " pass ", id, func() error {
		persistCalls++
		return nil
	})
	if err != nil {
		t.Fatalf("ImportIdentity failed: %v", err)
	}
	if identity.ID != "u1" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if persistCalls != 1 {
		t.Fatalf("persist calls = %d, want 1", persistCalls)
	}
}
