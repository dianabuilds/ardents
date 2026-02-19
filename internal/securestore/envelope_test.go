package securestore

import (
	"errors"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	data, err := Encrypt("pass", []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	plain, err := Decrypt("pass", data)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(plain) != "secret" {
		t.Fatalf("unexpected plaintext: %q", string(plain))
	}
}

func TestDecryptTamperedFailsDeterministically(t *testing.T) {
	data, err := Encrypt("pass", []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if len(data) < 10 {
		t.Fatalf("unexpected encrypted payload size: %d", len(data))
	}
	data[len(data)-2] ^= 0xFF
	_, err = Decrypt("pass", data)
	if !errors.Is(err, ErrAuthFailed) && !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestDecryptEnvelopeRejectsMalformedMetadata(t *testing.T) {
	enc, err := EncryptEnvelope("pass", []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt envelope failed: %v", err)
	}

	malformed := *enc
	malformed.Nonce = []byte{1, 2, 3}
	if _, err := DecryptEnvelope("pass", &malformed); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for malformed nonce, got %v", err)
	}
}

func TestDecryptEnvelopeRejectsKDFDowngrade(t *testing.T) {
	enc, err := EncryptEnvelope("pass", []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt envelope failed: %v", err)
	}

	downgraded := *enc
	downgraded.KDFTime = 1
	if _, err := DecryptEnvelope("pass", &downgraded); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for downgraded kdf params, got %v", err)
	}
}
