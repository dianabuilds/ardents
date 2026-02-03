package netdb

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/ids"
)

func TestStoreRouterInfoAndFindValue(t *testing.T) {
	db := New(DefaultRecordMaxTTLMs, DefaultK)
	ri := makeRouterInfo(t)
	b, err := EncodeRouterInfo(ri)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().UnixNano() / int64(time.Millisecond)
	status, code := db.Store(b, now)
	if status != "OK" || code != "" {
		t.Fatalf("expected OK, got %s %s", status, code)
	}
	key := dhtKey(TypeRouterInfo, ri.PeerID)
	val, ok := db.FindValue(key[:])
	if !ok || len(val) == 0 {
		t.Fatal("expected find_value to return record")
	}
}

func makeRouterInfo(t *testing.T) RouterInfo {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	peerID, err := ids.NewPeerID(pub)
	if err != nil {
		t.Fatal(err)
	}
	onionPub := make([]byte, 32)
	if _, err := rand.Read(onionPub); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().UnixNano() / int64(time.Millisecond)
	ri := RouterInfo{
		V:             1,
		PeerID:        peerID,
		TransportPub:  pub,
		OnionPub:      onionPub,
		Addrs:         []string{"quic://127.0.0.1:9999"},
		Caps:          RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: now,
		ExpiresAtMs:   now + 600_000,
	}
	unsigned, err := unsignedRouterBytes(ri)
	if err != nil {
		t.Fatal(err)
	}
	ri.Sig = ed25519.Sign(priv, unsigned)
	return ri
}
