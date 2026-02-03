package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/delivery"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
)

func TestSendChatAckOK(t *testing.T) {
	cfg := config.Default()
	rt := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	addr := rt.QUICAddr()
	if addr == "" {
		t.Fatal("missing QUIC addr")
	}
	ackBytes, err := rt.SendChat(ctx, "127.0.0.1:"+addr[strings.LastIndex(addr, ":")+1:], rt.PeerID(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(ackBytes) == 0 {
		t.Fatal("empty ack")
	}
	time.Sleep(50 * time.Millisecond)
	rec, ok := rt.tracker.Get(rt.tracker.LastID())
	if !ok || rec.Status != delivery.StatusAcked {
		t.Fatalf("expected acked, got %+v", rec)
	}
}
