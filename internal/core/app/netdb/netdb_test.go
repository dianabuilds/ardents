package netdb

import (
	"crypto/ed25519"
	"testing"

	"github.com/dianabuilds/ardents/internal/shared/testutil"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func TestStoreRouterInfoAndFindValue(t *testing.T) {
	db := New(DefaultRecordMaxTTLMs, DefaultK)
	now := timeutil.NowUnixMs()
	ri := makeRouterInfo(t, now)
	b, err := EncodeRouterInfo(ri)
	if err != nil {
		t.Fatal(err)
	}
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

func makeRouterInfo(t *testing.T, now int64) RouterInfo {
	t.Helper()
	priv, pub, peerID := testutil.NewPeerKeyAndID(t)
	onionPub := testutil.NewOnionPub(t)
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
