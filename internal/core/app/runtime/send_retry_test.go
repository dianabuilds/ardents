package runtime

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/delivery"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/transport/quic"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/testutil"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestSendEnvelopeWithRetryTimeout(t *testing.T) {
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

	srv, err := quic.NewServer(cfg)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		_ = srv.Stop(context.Background())
	}()

	rt := New(cfg)
	_, _, fromPeerID := testutil.NewPeerKeyAndID(t)
	msgID, err := uuidv7.New()
	if err != nil {
		t.Fatalf("msg id: %v", err)
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  "test.msg.v1",
		From:  envelope.From{PeerID: fromPeerID},
		To:    envelope.To{PeerID: srv.PeerID()},
		TSMs:  timeutil.NowUnixMs(),
		TTLMs: 1000,
	}
	envBytes, err := env.Encode()
	if err != nil {
		t.Fatalf("encode env: %v", err)
	}

	_, err = rt.sendEnvelopeWithRetry(context.Background(), srv.Addr(), srv.PeerID(), envBytes, 50*time.Millisecond, 2)
	if err == nil || err.Error() != ErrAckTimeout.Error() {
		t.Fatalf("expected %v, got %v", ErrAckTimeout, err)
	}
	rec, ok := rt.tracker.Get(msgID)
	if !ok || rec.Status != delivery.StatusFailed {
		t.Fatalf("expected failed delivery, got %#v", rec)
	}
}
