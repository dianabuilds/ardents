package discovery

import (
	"testing"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/testutil"
)

func TestResolverUsesBootstrapFirst(t *testing.T) {
	priv, pub, peerID := testutil.NewPeerKeyAndID(t)
	now := int64(1000)
	db := netdb.New(netdb.DefaultRecordMaxTTLMs, netdb.DefaultK)
	ri := netdb.RouterInfo{
		V:             1,
		PeerID:        peerID,
		TransportPub:  pub,
		OnionPub:      testutil.NewOnionPub(t),
		Addrs:         []string{"quic://127.0.0.1:9999"},
		Caps:          netdb.RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: now - 1,
		ExpiresAtMs:   now + 10_000,
	}
	signed, err := netdb.SignRouterInfo(priv, ri)
	if err != nil {
		t.Fatal(err)
	}
	b, err := netdb.EncodeRouterInfo(signed)
	if err != nil {
		t.Fatal(err)
	}
	if status, code := db.Store(b, now); status != "OK" {
		t.Fatalf("store routerinfo: %s", code)
	}

	resolver := Resolver{
		Bootstrap: []config.BootstrapPeer{{PeerID: peerID, Addrs: []string{"127.0.0.1:1111"}}},
		NetDB:     db,
		SessionAddr: func(id string) (string, bool) {
			return "127.0.0.1:2222", true
		},
	}
	addr, ok := resolver.ResolvePeerAddr(peerID, now)
	if !ok {
		t.Fatal("expected address")
	}
	if addr != "127.0.0.1:1111" {
		t.Fatalf("expected bootstrap addr, got %s", addr)
	}
}

func TestResolverFallsBackToNetDB(t *testing.T) {
	priv, pub, peerID := testutil.NewPeerKeyAndID(t)
	now := int64(1000)
	db := netdb.New(netdb.DefaultRecordMaxTTLMs, netdb.DefaultK)
	ri := netdb.RouterInfo{
		V:             1,
		PeerID:        peerID,
		TransportPub:  pub,
		OnionPub:      testutil.NewOnionPub(t),
		Addrs:         []string{"quic://127.0.0.1:9999"},
		Caps:          netdb.RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: now - 1,
		ExpiresAtMs:   now + 10_000,
	}
	signed, err := netdb.SignRouterInfo(priv, ri)
	if err != nil {
		t.Fatal(err)
	}
	b, err := netdb.EncodeRouterInfo(signed)
	if err != nil {
		t.Fatal(err)
	}
	if status, code := db.Store(b, now); status != "OK" {
		t.Fatalf("store routerinfo: %s", code)
	}

	resolver := Resolver{NetDB: db}
	addr, ok := resolver.ResolvePeerAddr(peerID, now)
	if !ok {
		t.Fatal("expected address")
	}
	if addr != "127.0.0.1:9999" {
		t.Fatalf("expected netdb addr, got %s", addr)
	}
}
