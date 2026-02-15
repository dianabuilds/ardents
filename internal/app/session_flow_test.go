package app

import (
	"testing"
	"time"

	"aim-chat/go-backend/internal/crypto"
)

func TestNormalizeSessionContactID(t *testing.T) {
	if got := NormalizeSessionContactID(" contact-1 "); got != "contact-1" {
		t.Fatalf("unexpected normalized contact id: %q", got)
	}
}

func TestEnsureVerifiedContact(t *testing.T) {
	if err := EnsureVerifiedContact(true); err != nil {
		t.Fatalf("unexpected error for verified contact: %v", err)
	}
	if err := EnsureVerifiedContact(false); err == nil {
		t.Fatal("expected error for unverified contact")
	}
}

func TestMapSessionState(t *testing.T) {
	now := time.Now().UTC()
	src := crypto.SessionState{
		SessionID:      "s1",
		ContactID:      "c1",
		PeerPublicKey:  []byte("peer-key"),
		SendChainIndex: 10,
		RecvChainIndex: 20,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	dst := MapSessionState(src)
	if dst.SessionID != "s1" || dst.ContactID != "c1" {
		t.Fatalf("unexpected mapped identifiers: %#v", dst)
	}
	if string(dst.PeerPublicKey) != "peer-key" {
		t.Fatalf("unexpected mapped peer key: %q", string(dst.PeerPublicKey))
	}
	src.PeerPublicKey[0] = 'X'
	if string(dst.PeerPublicKey) != "peer-key" {
		t.Fatal("peer key must be copied, not aliased")
	}
}
