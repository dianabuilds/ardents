package quic

import (
	"context"
	"crypto/tls"
	"os"
	"testing"
	"time"

	quicgo "github.com/quic-go/quic-go"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
)

func TestServerInboundLimit(t *testing.T) {
	tmp := t.TempDir()
	prevHome := os.Getenv("ARDENTS_HOME")
	if err := os.Setenv("ARDENTS_HOME", tmp); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("ARDENTS_HOME", prevHome)
	})

	cfg := config.Default()
	cfg.Listen.QUICAddr = "127.0.0.1:0"
	cfg.Limits.MaxInboundConns = 1

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	errCh := make(chan error, 1)
	srv.SetHandshakeErrorObserver(func(_ string, _ string, err error) {
		errCh <- err
	})
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		_ = srv.Stop(context.Background())
	}()

	keys, err := LoadOrCreateKeyMaterial("")
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{keys.TLSCert},
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}
	quicConf := &quicgo.Config{
		HandshakeIdleTimeout: 5 * time.Second,
		MaxIdleTimeout:       10 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn1, err := quicgo.DialAddr(ctx, srv.Addr(), tlsConf, quicConf)
	if err != nil {
		t.Fatalf("dial1: %v", err)
	}
	stream1, err := conn1.OpenStreamSync(ctx)
	if err != nil {
		_ = conn1.CloseWithError(0, "")
		t.Fatalf("stream1: %v", err)
	}
	defer func() {
		_ = stream1.Close()
		_ = conn1.CloseWithError(0, "")
	}()

	_, _ = quicgo.DialAddr(ctx, srv.Addr(), tlsConf, quicConf)

	select {
	case err := <-errCh:
		if err == nil || err.Error() != ErrMaxInboundConns.Error() {
			t.Fatalf("expected %v, got %v", ErrMaxInboundConns, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected inbound limit error")
	}
}
