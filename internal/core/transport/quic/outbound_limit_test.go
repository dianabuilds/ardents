package quic

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestDialerOutboundLimit(t *testing.T) {
	_ = withTempHome(t)

	cfg := config.Default()
	cfg.Listen.QUICAddr = "127.0.0.1:0"
	cfg.Limits.MaxOutboundConns = 1
	cfg.Limits.MaxInboundConns = 10

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		_ = srv.Stop(context.Background())
	}()

	d, err := NewDialer(cfg)
	if err != nil {
		t.Fatalf("new dialer: %v", err)
	}

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	fromID, err := ids.NewPeerID(pub)
	if err != nil {
		t.Fatalf("peer id: %v", err)
	}
	msgID, err := uuidv7.New()
	if err != nil {
		t.Fatalf("msg id: %v", err)
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  "test.msg.v1",
		From:  envelope.From{PeerID: fromID},
		To:    envelope.To{PeerID: srv.PeerID()},
		TSMs:  timeutil.NowUnixMs(),
		TTLMs: 1000,
	}
	envBytes, err := env.Encode()
	if err != nil {
		t.Fatalf("encode env: %v", err)
	}

	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		ctxA, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_, _ = d.SendEnvelope(ctxA, srv.Addr(), srv.PeerID(), envBytes, cfg.Limits.MaxMsgBytes)
		close(done)
	}()

	<-started
	time.Sleep(20 * time.Millisecond)

	ctxB, cancelB := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelB()
	_, err = d.SendEnvelope(ctxB, srv.Addr(), srv.PeerID(), envBytes, cfg.Limits.MaxMsgBytes)
	if !errors.Is(err, ErrMaxOutboundConns) {
		t.Fatalf("expected outbound limit error, got %v", err)
	}

	<-done
}
