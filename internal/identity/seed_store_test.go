package identity

import "testing"

func TestDecryptSeedRejectsMalformedEnvelope(t *testing.T) {
	env, err := EncryptSeed([]byte("seed-value"), []byte("password"))
	if err != nil {
		t.Fatalf("encrypt seed failed: %v", err)
	}

	malformed := *env
	malformed.Nonce = []byte{1, 2, 3}
	if _, err := DecryptSeed(&malformed, []byte("password")); err == nil {
		t.Fatal("expected error for malformed nonce")
	}
}

func TestDecryptSeedRejectsKDFDowngrade(t *testing.T) {
	env, err := EncryptSeed([]byte("seed-value"), []byte("password"))
	if err != nil {
		t.Fatalf("encrypt seed failed: %v", err)
	}

	downgraded := *env
	downgraded.KDFMemoryKB = 8 * 1024
	if _, err := DecryptSeed(&downgraded, []byte("password")); err == nil {
		t.Fatal("expected error for downgraded kdf policy")
	}
}
