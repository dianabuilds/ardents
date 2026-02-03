package garlic

import (
	"crypto/rand"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"
)

func TestGarlicEncryptDecrypt(t *testing.T) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		t.Fatal(err)
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	inner := Inner{
		V:           Version,
		ExpiresAtMs: time.Now().UTC().UnixNano()/int64(time.Millisecond) + 60_000,
		Cloves: []Clove{
			{Kind: "envelope", Envelope: []byte{0x01, 0x02}},
		},
	}
	msg, err := Encrypt("svc_test", pub, inner)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(mustEncode(t, msg))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decrypt(decoded, priv)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Cloves) != 1 || out.Cloves[0].Kind != "envelope" {
		t.Fatal("unexpected cloves")
	}
}

func TestGarlicDecryptWrongKey(t *testing.T) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		t.Fatal(err)
	}
	pub, _ := curve25519.X25519(priv, curve25519.Basepoint)
	inner := Inner{
		V:           Version,
		ExpiresAtMs: time.Now().UTC().UnixNano()/int64(time.Millisecond) + 60_000,
		Cloves: []Clove{
			{Kind: "envelope", Envelope: []byte{0x01, 0x02}},
		},
	}
	msg, err := Encrypt("svc_test", pub, inner)
	if err != nil {
		t.Fatal(err)
	}
	wrongPriv := make([]byte, 32)
	if _, err := rand.Read(wrongPriv); err != nil {
		t.Fatal(err)
	}
	_, err = Decrypt(msg, wrongPriv)
	if err == nil {
		t.Fatal("expected decrypt error")
	}
}

func mustEncode(t *testing.T, msg Message) []byte {
	t.Helper()
	b, err := Encode(msg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
