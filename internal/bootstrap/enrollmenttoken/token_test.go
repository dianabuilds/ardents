package enrollmenttoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func mustKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, prv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, prv
}

func baseClaims(now time.Time) Claims {
	return Claims{
		TokenID:          "tok-001",
		IssuedAt:         now.Add(-1 * time.Minute),
		ExpiresAt:        now.Add(10 * time.Minute),
		Scope:            "node.enroll",
		SubjectNodeGroup: "vps-default",
		Issuer:           RequiredIssuer,
		KeyID:            "issuer-k1",
	}
}

func TestVerifierRejectsInvalidIssuer(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	pub, prv := mustKeys(t)
	claims := baseClaims(now)
	claims.Issuer = "evil-issuer"
	token, err := EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	verifier := Verifier{
		RequiredIssuer: RequiredIssuer,
		PublicKeys:     map[string]ed25519.PublicKey{"issuer-k1": pub},
		Now:            func() time.Time { return now },
	}
	_, _, err = verifier.VerifyAndRedeem(token, NewInMemoryStore())
	if !errors.Is(err, ErrTokenIssuerInvalid) {
		t.Fatalf("expected ErrTokenIssuerInvalid, got %v", err)
	}
}

func TestVerifierRejectsMissingExpiresAt(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	pub, prv := mustKeys(t)
	claims := baseClaims(now)
	claims.ExpiresAt = time.Time{}
	token, err := EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	verifier := Verifier{
		RequiredIssuer: RequiredIssuer,
		PublicKeys:     map[string]ed25519.PublicKey{"issuer-k1": pub},
		Now:            func() time.Time { return now },
	}
	_, _, err = verifier.VerifyAndRedeem(token, NewInMemoryStore())
	if !errors.Is(err, ErrTokenClaimsInvalid) {
		t.Fatalf("expected ErrTokenClaimsInvalid, got %v", err)
	}
}

func TestVerifierRejectsInvalidScope(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	pub, prv := mustKeys(t)
	claims := baseClaims(now)
	claims.Scope = "node.admin"
	token, err := EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	verifier := Verifier{
		RequiredIssuer: RequiredIssuer,
		RequiredScope:  RequiredScope,
		PublicKeys:     map[string]ed25519.PublicKey{"issuer-k1": pub},
		Now:            func() time.Time { return now },
	}
	_, _, err = verifier.VerifyAndRedeem(token, NewInMemoryStore())
	if !errors.Is(err, ErrTokenScopeInvalid) {
		t.Fatalf("expected ErrTokenScopeInvalid, got %v", err)
	}
}

func TestVerifierRejectsReusedTokenID(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	pub, prv := mustKeys(t)
	claims := baseClaims(now)
	token, err := EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	store := NewInMemoryStore()
	verifier := Verifier{
		RequiredIssuer: RequiredIssuer,
		PublicKeys:     map[string]ed25519.PublicKey{"issuer-k1": pub},
		Now:            func() time.Time { return now },
	}
	if _, event, err := verifier.VerifyAndRedeem(token, store); err != nil {
		t.Fatalf("first redeem must pass, err=%v event=%+v", err, event)
	}
	_, _, err = verifier.VerifyAndRedeem(token, store)
	if !errors.Is(err, ErrTokenAlreadyUsed) {
		t.Fatalf("expected ErrTokenAlreadyUsed, got %v", err)
	}
}

func TestVerifierRejectsBadSignature(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	pub, prv := mustKeys(t)
	claims := baseClaims(now)
	token, err := EncodeSignedToken(claims, prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	parts := []byte(token)
	parts[len(parts)-1] = 'A'
	token = string(parts)
	verifier := Verifier{
		RequiredIssuer: RequiredIssuer,
		PublicKeys:     map[string]ed25519.PublicKey{"issuer-k1": pub},
		Now:            func() time.Time { return now },
	}
	_, _, err = verifier.VerifyAndRedeem(token, NewInMemoryStore())
	if !errors.Is(err, ErrTokenSignatureInvalid) {
		t.Fatalf("expected ErrTokenSignatureInvalid, got %v", err)
	}
}

func TestFileStorePersistsRedemption(t *testing.T) {
	path := t.TempDir() + "/redeemed.json"
	store := NewFileStore(path)
	if err := store.Bootstrap(); err != nil {
		t.Fatalf("bootstrap store: %v", err)
	}
	at := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	ok, err := store.TryRedeem("tok-001", at)
	if err != nil || !ok {
		t.Fatalf("first redeem must pass, ok=%v err=%v", ok, err)
	}

	storeReloaded := NewFileStore(path)
	if err := storeReloaded.Bootstrap(); err != nil {
		t.Fatalf("bootstrap reloaded store: %v", err)
	}
	ok, err = storeReloaded.TryRedeem("tok-001", at.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("second redeem error: %v", err)
	}
	if ok {
		t.Fatal("expected persisted token_id to be rejected as already used")
	}
}

func TestEncodeSignedTokenFormat(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	_, prv := mustKeys(t)
	token, err := EncodeSignedToken(baseClaims(now), prv)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("expected 2 token parts, got %d", len(parts))
	}
	if _, err := base64.RawURLEncoding.DecodeString(parts[0]); err != nil {
		t.Fatalf("payload is not base64url: %v", err)
	}
	if _, err := base64.RawURLEncoding.DecodeString(parts[1]); err != nil {
		t.Fatalf("signature is not base64url: %v", err)
	}
}
