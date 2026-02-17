package policy

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

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
