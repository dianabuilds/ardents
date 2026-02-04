package main

import (
	"errors"
	"testing"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
)

func TestLoadClientConfigMissingReturnsBootstrapRequired(t *testing.T) {
	home := t.TempDir()
	_, err := loadClientConfig(home, "")
	if !errors.Is(err, errClientBootstrapRequired) {
		t.Fatalf("expected ERR_CLIENT_BOOTSTRAP_REQUIRED, got %v", err)
	}
}

func TestOrderClientPeersPrefersIdentityMatch(t *testing.T) {
	in := []config.ClientPeer{
		{PeerID: "p1", Addrs: []string{"a1"}, IdentityID: "did:key:zzz"},
		{PeerID: "p2", Addrs: []string{"a2"}, IdentityID: "did:key:aaa"},
		{PeerID: "p3", Addrs: []string{"a3"}, IdentityID: ""},
	}
	out := orderClientPeers(in, "did:key:aaa")
	if len(out) != len(in) {
		t.Fatalf("unexpected len: %d", len(out))
	}
	if out[0].PeerID != "p2" {
		t.Fatalf("expected p2 first, got %s", out[0].PeerID)
	}
}
