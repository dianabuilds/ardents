package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

func TestDeriveKeysDeterministic(t *testing.T) {
	seed := []byte("test-seed-material")
	k1, err := DeriveKeys(seed)
	if err != nil {
		t.Fatalf("derive keys 1 failed: %v", err)
	}
	k2, err := DeriveKeys(seed)
	if err != nil {
		t.Fatalf("derive keys 2 failed: %v", err)
	}
	if !bytes.Equal(k1.SigningPublicKey, k2.SigningPublicKey) {
		t.Fatal("signing public keys should be deterministic")
	}
	if !bytes.Equal(k1.EncryptionSeed, k2.EncryptionSeed) {
		t.Fatal("encryption seeds should be deterministic")
	}
}

func TestEncryptDecryptSeed(t *testing.T) {
	seed := []byte("mnemonic-bytes-placeholder")
	password := []byte("strong-password")

	env, err := EncryptSeed(seed, password)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	got, err := DecryptSeed(env, password)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if !bytes.Equal(seed, got) {
		t.Fatal("decrypted seed mismatch")
	}
}

func TestBuildIdentityIDAndVerify(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	id, err := BuildIdentityID(pub)
	if err != nil {
		t.Fatalf("build id failed: %v", err)
	}
	if !strings.HasPrefix(id, "aim1") {
		t.Fatalf("identity id must have aim1 prefix, got: %s", id)
	}
	ok, err := VerifyIdentityID(id, pub)
	if err != nil {
		t.Fatalf("verify id failed: %v", err)
	}
	if !ok {
		t.Fatal("identity id verification should pass")
	}
}

func TestContactCardSignVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	id, err := BuildIdentityID(pub)
	if err != nil {
		t.Fatalf("build id failed: %v", err)
	}
	card, err := SignContactCard(id, "alice", pub, priv)
	if err != nil {
		t.Fatalf("sign card failed: %v", err)
	}
	ok, err := VerifyContactCard(card)
	if err != nil {
		t.Fatalf("verify card failed: %v", err)
	}
	if !ok {
		t.Fatal("signed contact card should verify")
	}
}
