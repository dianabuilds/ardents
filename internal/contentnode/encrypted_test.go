package contentnode

import (
	"testing"

	"github.com/dianabuilds/ardents/internal/shared/identity"
)

func TestEncryptDecryptNode(t *testing.T) {
	owner, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ownerID := owner.ID
	recipientID := recipient.ID
	links := []Link{{Rel: "prev", NodeID: "cidv1-abc"}}
	body := map[string]any{"v": uint64(1), "note": "secret"}
	n, err := EncryptNode(ownerID, owner.PrivateKey, "doc.note.v1", links, body, []string{recipientID})
	if err != nil {
		t.Fatal(err)
	}
	if n.Type != "enc.node.v1" || len(n.Links) != 0 {
		t.Fatalf("expected encrypted node wrapper")
	}
	payload, err := DecryptNode(n, recipientID, recipient.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Type != "doc.note.v1" || len(payload.Links) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Links[0].Rel != "prev" {
		t.Fatalf("unexpected links")
	}
}

func TestRecipientsSorted(t *testing.T) {
	owner, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r1, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r2, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ownerID := owner.ID
	id1 := r1.ID
	id2 := r2.ID
	recipients := []string{id2, id1}
	n, err := EncryptNode(ownerID, owner.PrivateKey, "doc.note.v1", []Link{}, map[string]any{"v": uint64(1)}, recipients)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := n.Body.(EncryptedBody)
	if !ok {
		t.Fatalf("unexpected body type")
	}
	if len(body.Recipients) != 2 {
		t.Fatalf("expected 2 recipients")
	}
	if body.Recipients[0].IdentityID > body.Recipients[1].IdentityID {
		t.Fatalf("recipients not sorted")
	}
}

func TestDecryptNoRecipient(t *testing.T) {
	owner, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ownerID := owner.ID
	recipientID := recipient.ID
	n, err := EncryptNode(ownerID, owner.PrivateKey, "doc.note.v1", []Link{}, map[string]any{"v": uint64(1)}, []string{recipientID})
	if err != nil {
		t.Fatal(err)
	}
	other, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptNode(n, other.ID, other.PrivateKey); err == nil {
		t.Fatalf("expected error")
	}
}
