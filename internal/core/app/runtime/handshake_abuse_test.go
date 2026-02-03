package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
)

func TestHandshakeAbuseBan(t *testing.T) {
	cfg := config.Default()
	cfg.Limits.BanWindowMs = int64((1 * time.Minute) / time.Millisecond)
	rt := New(cfg)

	for i := 0; i < 5; i++ {
		rt.observeHandshakeError("peer_x", "", errors.New("ERR_HANDSHAKE_TIME_SKEW"))
	}
	if !rt.IsBanned("peer_x") {
		t.Fatal("expected peer to be banned after handshake abuse")
	}
}
