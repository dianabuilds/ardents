package runtime

import (
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/aichat"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func TestAIChatTaskFlow(t *testing.T) {
	rt := newTestRuntime(t)
	input := map[string]any{
		"v": uint64(1),
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
		"policy": map[string]any{
			"visibility": "public",
		},
	}
	req := tasks.Request{
		V:               tasks.Version,
		TaskID:          mustUUID(t),
		ClientRequestID: mustUUID(t),
		JobType:         "ai.chat.v1",
		Input:           input,
		TSMs:            timeutil.NowUnixMs(),
	}
	payload, err := tasks.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: mustUUID(t),
		Type:  tasks.RequestType,
		From: envelope.From{
			PeerID:     rt.peerID,
			IdentityID: rt.identity.ID,
		},
		To: envelope.To{
			PeerID: rt.peerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	if err := env.Sign(rt.identity.PrivateKey); err != nil {
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
	if len(resps) < 3 {
		t.Fatalf("expected ack+accept+result")
	}
	ackEnv, err := envelope.DecodeEnvelope(resps[0])
	if err != nil {
		t.Fatal(err)
	}
	ap, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if ap.Status != "OK" {
		t.Fatalf("expected OK ack")
	}
	resultEnv, err := envelope.DecodeEnvelope(resps[2])
	if err != nil {
		t.Fatal(err)
	}
	resultPayload, err := tasks.DecodeResult(resultEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	nodeBytes, err := rt.store.Get(resultPayload.ResultNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := contentnode.VerifyBytes(nodeBytes, resultPayload.ResultNodeID); err != nil {
		t.Fatal(err)
	}
	var node contentnode.Node
	if err := contentnode.Decode(nodeBytes, &node); err != nil {
		t.Fatal(err)
	}
	if node.Type != "ai.chat.transcript.v1" {
		t.Fatalf("unexpected node type")
	}
	if inputObj, err := aichat.DecodeInput(req.Input, rt.cfg.Limits.MaxPayloadBytes); err == nil {
		if inputObj.Policy.Visibility != "public" {
			t.Fatalf("unexpected policy")
		}
	}
}
