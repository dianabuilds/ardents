package quic

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func TestHandshakeHint_RouterInfoIsDeliveredToDialerObserver(t *testing.T) {
	_ = withTempHome(t)

	cfg := config.Default()
	cfg.Listen.QUICAddr = "127.0.0.1:0"

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		_ = srv.Stop(context.Background())
	})

	quicAddr, err := ParseQUICAddr(srv.Addr())
	if err != nil {
		t.Fatalf("parse quic addr: %v", err)
	}
	var onionPub [32]byte
	if _, err := rand.Read(onionPub[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	nowMs := timeutil.NowUnixMs()
	rec := netdb.RouterInfo{
		V:             1,
		PeerID:        srv.peerID,
		TransportPub:  srv.keys.PublicKey,
		OnionPub:      onionPub[:],
		Addrs:         []string{quicAddr},
		Caps:          netdb.RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: nowMs,
		ExpiresAtMs:   nowMs + int64((10 * time.Minute / time.Millisecond)),
	}
	signed, err := netdb.SignRouterInfo(srv.keys.PrivateKey, rec)
	if err != nil {
		t.Fatalf("sign routerinfo: %v", err)
	}
	hintBytes, err := netdb.EncodeRouterInfo(signed)
	if err != nil {
		t.Fatalf("encode routerinfo: %v", err)
	}
	srv.SetRouterInfoHint(hintBytes)

	d, err := NewDialer(cfg)
	if err != nil {
		t.Fatalf("new dialer: %v", err)
	}

	var (
		mu   sync.Mutex
		seen []byte
	)
	d.SetHandshakeHintObserver(func(peerID string, routerInfoBytes []byte) {
		if peerID != srv.PeerID() {
			return
		}
		mu.Lock()
		seen = append([]byte(nil), routerInfoBytes...)
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := d.DialAndHandshake(ctx, srv.Addr(), srv.PeerID()); err != nil {
		t.Fatalf("dial and handshake: %v", err)
	}

	mu.Lock()
	got := append([]byte(nil), seen...)
	mu.Unlock()
	if len(got) == 0 {
		t.Fatal("expected handshake hint")
	}
	if string(got) != string(hintBytes) {
		t.Fatal("unexpected handshake hint bytes")
	}
}
