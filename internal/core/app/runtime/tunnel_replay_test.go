package runtime

import (
	crand "crypto/rand"
	"testing"

	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/core/infra/observability"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func TestHandleTunnelDataReplay(t *testing.T) {
	r := &Runtime{log: observability.New()}
	key := make([]byte, 32)
	if _, err := crand.Read(key); err != nil {
		t.Fatalf("key: %v", err)
	}
	tunnelID := make([]byte, 16)
	if _, err := crand.Read(tunnelID); err != nil {
		t.Fatalf("tunnel id: %v", err)
	}
	nowMs := timeutil.NowUnixMs()
	sess := &tunnelSession{key: key, expiresAtMs: nowMs + 60_000}
	r.tunnels = map[string]*tunnelSession{r.tunnelKey(tunnelID): sess}

	inner := tunnel.Inner{V: 1, Kind: "padding"}
	ct, err := tunnel.EncryptData(key, inner)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	data := tunnel.Data{V: 1, TunnelID: tunnelID, Seq: 1, CT: ct}
	payload, err := tunnel.EncodeData(data)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	_, _ = r.handleTunnelData("peerA", payload)
	if sess.lastSeq != 1 {
		t.Fatalf("expected lastSeq=1, got %d", sess.lastSeq)
	}
	_, _ = r.handleTunnelData("peerA", payload)
	if sess.lastSeq != 1 {
		t.Fatalf("expected replay to keep lastSeq=1, got %d", sess.lastSeq)
	}
}
