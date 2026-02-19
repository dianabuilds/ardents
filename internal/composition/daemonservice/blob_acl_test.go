package daemonservice

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/pkg/models"
)

func newBlobACLPair(t *testing.T) (*Service, *Service) {
	t.Helper()
	registry := newBlobProviderRegistry()
	cfg := newMockConfig()
	owner, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new owner: %v", err)
	}
	receiver, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new receiver: %v", err)
	}
	useSharedBlobProviders(registry, owner, receiver)
	if _, _, err := owner.CreateIdentity("owner-pass"); err != nil {
		t.Fatalf("create owner identity: %v", err)
	}
	if _, _, err := receiver.CreateIdentity("receiver-pass"); err != nil {
		t.Fatalf("create receiver identity: %v", err)
	}
	now := time.Now().UTC()
	if err := owner.bindingStore.Upsert(models.NodeBindingRecord{
		IdentityID: owner.localPeerID(),
		NodeID:     "node-owner",
		BoundAt:    now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("upsert binding: %v", err)
	}
	startBlobNetworking(t, owner, receiver)
	cleanupBlobNetworking(t, owner, receiver)
	return owner, receiver
}

func TestBlobACLFetchDeniedForNonContactWhenBound(t *testing.T) {
	owner, receiver := newBlobACLPair(t)
	if _, err := owner.SetBlobACLPolicy("owner_contacts", nil); err != nil {
		t.Fatalf("set acl: %v", err)
	}

	meta, err := owner.PutAttachment("a.txt", "text/plain", base64.StdEncoding.EncodeToString([]byte("payload")))
	if err != nil {
		t.Fatalf("put attachment: %v", err)
	}

	_, _, err = receiver.GetAttachment(meta.ID)
	if !errors.Is(err, contracts.ErrAttachmentAccessDenied) {
		t.Fatalf("expected access denied, got: %v", err)
	}
}

func TestBlobACLFetchAllowedForContactWhenBound(t *testing.T) {
	owner, receiver := newBlobACLPair(t)
	if err := owner.AddContact(receiver.localPeerID(), "Receiver"); err != nil {
		t.Fatalf("add contact: %v", err)
	}
	if _, err := owner.SetBlobACLPolicy("owner_contacts", nil); err != nil {
		t.Fatalf("set acl: %v", err)
	}

	meta, err := owner.PutAttachment("a.txt", "text/plain", base64.StdEncoding.EncodeToString([]byte("payload")))
	if err != nil {
		t.Fatalf("put attachment: %v", err)
	}

	_, data, err := receiver.GetAttachment(meta.ID)
	if err != nil {
		t.Fatalf("expected fetch success, got: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected payload: %q", string(data))
	}
}

func TestBlobACLAllowlistRuntimeUpdateWithoutRestart(t *testing.T) {
	owner, receiver := newBlobACLPair(t)
	if _, err := owner.SetBlobACLPolicy("allowlist", nil); err != nil {
		t.Fatalf("set acl: %v", err)
	}

	meta, err := owner.PutAttachment("a.txt", "text/plain", base64.StdEncoding.EncodeToString([]byte("payload")))
	if err != nil {
		t.Fatalf("put attachment: %v", err)
	}

	if _, _, err := receiver.GetAttachment(meta.ID); !errors.Is(err, contracts.ErrAttachmentAccessDenied) {
		t.Fatalf("expected access denied before allowlist update, got: %v", err)
	}

	if _, err := owner.SetBlobACLPolicy("allowlist", []string{receiver.localPeerID()}); err != nil {
		t.Fatalf("set acl allowlist: %v", err)
	}

	_, data, err := receiver.GetAttachment(meta.ID)
	if err != nil {
		t.Fatalf("expected fetch success after allowlist update, got: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected payload: %q", string(data))
	}
}
