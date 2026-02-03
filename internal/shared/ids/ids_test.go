package ids

import (
	"crypto/ed25519"
	"testing"
)

func TestNewServiceID(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	identityID, err := NewIdentityID(pub)
	if err != nil {
		t.Fatal(err)
	}
	serviceID, err := NewServiceID(identityID, "demo.v1")
	if err != nil {
		t.Fatal(err)
	}
	if serviceID == "" {
		t.Fatal("expected service id")
	}
}

func TestNewChannelID(t *testing.T) {
	id, err := NewChannelID([]byte{0x01, 0x02, 0x03})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected channel id")
	}
}
