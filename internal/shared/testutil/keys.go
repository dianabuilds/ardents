package testutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/dianabuilds/ardents/internal/shared/ids"
)

func NewPeerKeyAndID(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := ids.NewPeerID(pub)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub, peerID
}

func NewOnionPub(t *testing.T) []byte {
	t.Helper()
	onionPub := make([]byte, 32)
	if _, err := rand.Read(onionPub); err != nil {
		t.Fatal(err)
	}
	return onionPub
}
