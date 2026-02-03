package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestPipeline_TTLExpired(t *testing.T) {
	rt := newTestRuntime(t)
	env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	env.TSMs = 1
	env.TTLMs = 1
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.Status != "REJECTED" || p.ErrorCode != "ERR_TTL_EXPIRED" {
		t.Fatalf("unexpected ack: %+v", p)
	}
}

func TestPipeline_Dedup(t *testing.T) {
	rt := newTestRuntime(t)
	env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.handleEnvelope("peer_x", data); err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.Status != "DUPLICATE" {
		t.Fatalf("expected DUPLICATE, got %+v", p)
	}
}

func TestPipeline_SigRequired(t *testing.T) {
	rt := newTestRuntime(t)
	env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	env.From.IdentityID = "did:key:z6Mku7wq4aVxK8rV8b1eYB2R7v9P7S5i1r6Y3g6x2z6xQx"
	env.Sig = nil
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.ErrorCode != "ERR_SIG_REQUIRED" {
		t.Fatalf("expected ERR_SIG_REQUIRED, got %+v", p)
	}
}

func TestPipeline_SigInvalid(t *testing.T) {
	rt := newTestRuntime(t)
	id, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	env.From.IdentityID = id.ID
	if err := env.Sign(id.PrivateKey); err != nil {
		t.Fatal(err)
	}
	env.Payload = []byte{0x02}
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.ErrorCode != "ERR_SIG_INVALID" {
		t.Fatalf("expected ERR_SIG_INVALID, got %+v", p)
	}
}

func TestPipeline_PowRequired(t *testing.T) {
	rt := newTestRuntime(t)
	env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	env.From.IdentityID = ""
	env.Pow = nil
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.ErrorCode != pow.ErrPowRequired.Error() {
		t.Fatalf("expected ERR_POW_REQUIRED, got %+v", p)
	}
}

func TestPipeline_PowInvalid(t *testing.T) {
	rt := newTestRuntime(t)
	env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	env.From.IdentityID = ""
	env.Pow = &pow.Stamp{
		V:          1,
		Difficulty: 20,
		Nonce:      make([]byte, 15),
		Subject:    make([]byte, 32),
	}
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.ErrorCode != pow.ErrPowInvalid.Error() {
		t.Fatalf("expected ERR_POW_INVALID, got %+v", p)
	}
}

func TestPipeline_PowAbuseBan(t *testing.T) {
	rt := newTestRuntime(t)
	for i := 0; i < 5; i++ {
		env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01})
		if err != nil {
			t.Fatal(err)
		}
		env.From.IdentityID = ""
		env.Pow = &pow.Stamp{
			V:          1,
			Difficulty: 20,
			Nonce:      make([]byte, 15),
			Subject:    make([]byte, 32),
		}
		data, err := env.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rt.handleEnvelope("peer_x", data); err != nil {
			t.Fatal(err)
		}
	}
	if !rt.IsBanned("peer_x") {
		t.Fatalf("expected peer_x to be banned after repeated PoW errors")
	}
}

func TestPipeline_RevokedIdentity(t *testing.T) {
	rt := newTestRuntime(t)
	id, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rt.book.RevokedIDs = []string{id.ID}
	env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	env.From.IdentityID = id.ID
	if err := env.Sign(id.PrivateKey); err != nil {
		t.Fatal(err)
	}
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.ErrorCode != "ERR_ID_REVOKED" {
		t.Fatalf("expected ERR_ID_REVOKED, got %+v", p)
	}
}

func TestPipeline_PayloadTooLarge(t *testing.T) {
	cfg := config.Default()
	cfg.Limits.MaxPayloadBytes = 4
	rt := New(cfg)
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = rt.Stop(context.Background())
	})
	env, err := buildEnv(rt, "chat.msg.v1", []byte{0x01, 0x02, 0x03, 0x04, 0x05})
	if err != nil {
		t.Fatal(err)
	}
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.ErrorCode != "ERR_PAYLOAD_TOO_LARGE" {
		t.Fatalf("expected ERR_PAYLOAD_TOO_LARGE, got %+v", p)
	}
}

func TestPipeline_UnsupportedType(t *testing.T) {
	rt := newTestRuntime(t)
	env, err := buildEnv(rt, "unknown.v1", []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	sub := pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
	stamp, err := pow.Generate(sub, rt.cfg.Pow.DefaultDifficulty)
	if err != nil {
		t.Fatal(err)
	}
	env.Pow = stamp
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	p := decodeAck(t, resps[0])
	if p.Status != "REJECTED" || p.ErrorCode != "ERR_UNSUPPORTED_TYPE" {
		t.Fatalf("unexpected ack: %+v", p)
	}
}

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	cfg := config.Default()
	cfg.Observability.HealthAddr = freeAddr(t)
	cfg.Observability.MetricsAddr = freeAddr(t)
	rt := New(cfg)
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = rt.Stop(context.Background())
	})
	return rt
}

func buildEnv(rt *Runtime, typ string, payload []byte) (envelope.Envelope, error) {
	msgID, err := uuidv7.New()
	if err != nil {
		return envelope.Envelope{}, err
	}
	return envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  typ,
		From: envelope.From{
			PeerID: rt.PeerID(),
		},
		To: envelope.To{
			PeerID: "peer_bafkrw2xqkvpw3ehny6qo2rwcynjb36zl24d3p5y3g6g6qg4keq3f",
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}, nil
}

func decodeAck(t *testing.T, data []byte) ack.Payload {
	t.Helper()
	env, err := envelope.DecodeEnvelope(data)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ack.Decode(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
