package daemonservice

import (
	"aim-chat/go-backend/internal/bootstrap/enrollmenttoken"
	"aim-chat/go-backend/internal/waku"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func mustIssuerPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, prv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}
	return pub, prv
}

func TestRedeemEnrollmentTokenExpiredRejected(t *testing.T) {
	pub, prv := mustIssuerPair(t)
	t.Setenv("AIM_ENROLLMENT_ISSUER_KEYS", "issuer-k1:"+base64.StdEncoding.EncodeToString(pub))
	svc, err := NewServiceForDaemonWithDataDir(waku.DefaultConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("create daemon service: %v", err)
	}
	claims := enrollmenttoken.Claims{
		TokenID:          "tok-expired",
		IssuedAt:         time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt:        time.Now().UTC().Add(-1 * time.Minute),
		Scope:            "node.enroll",
		SubjectNodeGroup: "default",
		Issuer:           enrollmenttoken.RequiredIssuer,
		KeyID:            "issuer-k1",
	}
	token, err := enrollmenttoken.EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	_, err = svc.RedeemEnrollmentToken(token)
	if !errors.Is(err, enrollmenttoken.ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestRedeemEnrollmentTokenSingleUse(t *testing.T) {
	pub, prv := mustIssuerPair(t)
	t.Setenv("AIM_ENROLLMENT_ISSUER_KEYS", "issuer-k1:"+base64.StdEncoding.EncodeToString(pub))
	svc, err := NewServiceForDaemonWithDataDir(waku.DefaultConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("create daemon service: %v", err)
	}
	claims := enrollmenttoken.Claims{
		TokenID:          "tok-single-use",
		IssuedAt:         time.Now().UTC().Add(-1 * time.Minute),
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute),
		Scope:            "node.enroll",
		SubjectNodeGroup: "default",
		Issuer:           enrollmenttoken.RequiredIssuer,
		KeyID:            "issuer-k1",
	}
	token, err := enrollmenttoken.EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	first, err := svc.RedeemEnrollmentToken(token)
	if err != nil {
		t.Fatalf("first redeem failed: %v", err)
	}
	if first.TokenID != claims.TokenID {
		t.Fatalf("unexpected first redeem token_id: %q", first.TokenID)
	}
	_, err = svc.RedeemEnrollmentToken(token)
	if !errors.Is(err, enrollmenttoken.ErrTokenAlreadyUsed) {
		t.Fatalf("expected ErrTokenAlreadyUsed, got %v", err)
	}
}

func TestRedeemEnrollmentTokenInvalidIssuerRejected(t *testing.T) {
	pub, prv := mustIssuerPair(t)
	t.Setenv("AIM_ENROLLMENT_ISSUER_KEYS", "issuer-k1:"+base64.StdEncoding.EncodeToString(pub))
	svc, err := NewServiceForDaemonWithDataDir(waku.DefaultConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("create daemon service: %v", err)
	}
	claims := enrollmenttoken.Claims{
		TokenID:          "tok-issuer",
		IssuedAt:         time.Now().UTC().Add(-1 * time.Minute),
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute),
		Scope:            "node.enroll",
		SubjectNodeGroup: "default",
		Issuer:           "evil-issuer",
		KeyID:            "issuer-k1",
	}
	token, err := enrollmenttoken.EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	_, err = svc.RedeemEnrollmentToken(token)
	if !errors.Is(err, enrollmenttoken.ErrTokenIssuerInvalid) {
		t.Fatalf("expected ErrTokenIssuerInvalid, got %v", err)
	}
}
