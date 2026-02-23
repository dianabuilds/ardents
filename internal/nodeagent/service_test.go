package nodeagent

import (
	"aim-chat/go-backend/internal/bootstrap/enrollmenttoken"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func mustKP(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, prv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, prv
}

func TestInitIdempotent(t *testing.T) {
	svc := New(t.TempDir())
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	first, created, err := svc.Init()
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if !created {
		t.Fatal("expected created=true on first init")
	}
	second, created, err := svc.Init()
	if err != nil {
		t.Fatalf("second init failed: %v", err)
	}
	if created {
		t.Fatal("expected created=false on second init")
	}
	if first.NodeID != second.NodeID {
		t.Fatalf("expected same node id, got %q vs %q", first.NodeID, second.NodeID)
	}
}

func TestEnrollRejectsInvalidIssuer(t *testing.T) {
	svc := New(t.TempDir())
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	if _, _, err := svc.Init(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	pub, prv := mustKP(t)
	claims := enrollmenttoken.Claims{
		TokenID:          "tok-1",
		IssuedAt:         now.Add(-1 * time.Minute),
		ExpiresAt:        now.Add(10 * time.Minute),
		Scope:            "node.enroll",
		SubjectNodeGroup: "default",
		Issuer:           "invalid-issuer",
		KeyID:            "issuer-k1",
	}
	token, err := enrollmenttoken.EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	_, err = svc.Enroll(token, map[string]ed25519.PublicKey{"issuer-k1": pub})
	if !errors.Is(err, enrollmenttoken.ErrTokenIssuerInvalid) {
		t.Fatalf("expected ErrTokenIssuerInvalid, got %v", err)
	}
}

func TestEnrollSingleUseToken(t *testing.T) {
	svc := New(t.TempDir())
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	if _, _, err := svc.Init(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	pub, prv := mustKP(t)
	claims := enrollmenttoken.Claims{
		TokenID:          "tok-1",
		IssuedAt:         now.Add(-1 * time.Minute),
		ExpiresAt:        now.Add(10 * time.Minute),
		Scope:            "node.enroll",
		SubjectNodeGroup: "default",
		Issuer:           enrollmenttoken.RequiredIssuer,
		KeyID:            "issuer-k1",
	}
	token, err := enrollmenttoken.EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	if _, err := svc.Enroll(token, map[string]ed25519.PublicKey{"issuer-k1": pub}); err != nil {
		t.Fatalf("first enroll failed: %v", err)
	}
	if _, err := svc.Enroll(token, map[string]ed25519.PublicKey{"issuer-k1": pub}); !errors.Is(err, enrollmenttoken.ErrTokenAlreadyUsed) {
		t.Fatalf("expected ErrTokenAlreadyUsed, got %v", err)
	}
}

func TestStatusContainsHealthAndPeerData(t *testing.T) {
	svc := New(t.TempDir())
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	if _, _, err := svc.Init(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	status, err := svc.Status(context.TODO(), "", "")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Health == "" {
		t.Fatal("health must be present")
	}
	if status.ProfileID != "network_assist_default" {
		t.Fatalf("unexpected profile id: %q", status.ProfileID)
	}
	if status.PeerCount < 0 {
		t.Fatalf("peer count must be >=0, got %d", status.PeerCount)
	}
}
