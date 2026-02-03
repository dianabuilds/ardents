package relay

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestPacketSealOpen(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	inner := []byte("hello-relay")
	pkt, err := Build("peer_x", 1000, inner, pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(pkt); err != nil {
		t.Fatal(err)
	}
	opened, err := OpenInner(pub, priv, pkt.Inner)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(opened, inner) {
		t.Fatalf("inner mismatch")
	}
}
