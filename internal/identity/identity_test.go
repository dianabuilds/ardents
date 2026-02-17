package identity

import (
	"bytes"
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
