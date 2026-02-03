package runtime

import (
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestTaskIdempotencySamePayload(t *testing.T) {
	rt := newTestRuntime(t)
	req := tasks.Request{
		V:               tasks.Version,
		TaskID:          mustUUID(t),
		ClientRequestID: mustUUID(t),
		JobType:         "ai.chat.v1",
		Input:           map[string]any{"v": uint64(1)},
		TSMs:            timeutil.NowUnixMs(),
	}
	payload, err := tasks.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	data := buildTaskEnv(t, rt, payload)
	resps1, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	data2 := buildTaskEnv(t, rt, payload)
	resps2, err := rt.handleEnvelope("peer_x", data2)
	if err != nil {
		t.Fatal(err)
	}
	if len(resps1) != len(resps2) {
		t.Fatalf("expected same response count")
	}
}

func TestTaskRejectOnDifferentPayload(t *testing.T) {
	rt := newTestRuntime(t)
	clientReq := mustUUID(t)
	taskID := mustUUID(t)
	req := tasks.Request{
		V:               tasks.Version,
		TaskID:          taskID,
		ClientRequestID: clientReq,
		JobType:         "ai.chat.v1",
		Input:           map[string]any{"v": uint64(1)},
		TSMs:            timeutil.NowUnixMs(),
	}
	payload, _ := tasks.EncodeRequest(req)
	env := buildTaskEnv(t, rt, payload)
	resps1, err := rt.handleEnvelope("peer_x", env)
	if err != nil || len(resps1) == 0 {
		t.Fatal("expected response")
	}
	req2 := req
	req2.Input = map[string]any{"v": uint64(2)}
	payload2, _ := tasks.EncodeRequest(req2)
	env2 := buildTaskEnv(t, rt, payload2)
	resps2, err := rt.handleEnvelope("peer_x", env2)
	if err != nil || len(resps2) == 0 {
		t.Fatal("expected response")
	}
	ackEnv, err := envelope.DecodeEnvelope(resps2[0])
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
	if len(resps2) < 2 {
		t.Fatalf("expected fail response")
	}
	assertTaskFail(t, resps2[1], tasks.ErrTaskRejected.Error())
}

func buildTaskEnv(t *testing.T, rt *Runtime, payload []byte) []byte {
	t.Helper()
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
	return data
}

func TestTaskRejectOnDuplicateTaskID(t *testing.T) {
	rt := newTestRuntime(t)
	taskID := mustUUID(t)
	req := tasks.Request{
		V:               tasks.Version,
		TaskID:          taskID,
		ClientRequestID: mustUUID(t),
		JobType:         "ai.chat.v1",
		Input:           map[string]any{"v": uint64(1)},
		TSMs:            timeutil.NowUnixMs(),
	}
	payload, _ := tasks.EncodeRequest(req)
	env := buildTaskEnv(t, rt, payload)
	if _, err := rt.handleEnvelope("peer_x", env); err != nil {
		t.Fatal(err)
	}
	req2 := req
	req2.ClientRequestID = mustUUID(t)
	payload2, _ := tasks.EncodeRequest(req2)
	env2 := buildTaskEnv(t, rt, payload2)
	resps, err := rt.handleEnvelope("peer_x", env2)
	if err != nil || len(resps) < 2 {
		t.Fatal("expected responses")
	}
	assertTaskFail(t, resps[1], tasks.ErrTaskRejected.Error())
}

func mustUUID(t *testing.T) string {
	t.Helper()
	id, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertTaskFail(t *testing.T, envBytes []byte, expectedCode string) {
	t.Helper()
	failEnv, err := envelope.DecodeEnvelope(envBytes)
	if err != nil {
		t.Fatal(err)
	}
	failPayload, err := tasks.DecodeFail(failEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if failPayload.ErrorCode != expectedCode {
		t.Fatalf("expected %s", expectedCode)
	}
}
