package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/services/dirquery"
	"github.com/dianabuilds/ardents/internal/core/app/services/servicedesc"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestDirQueryV2ReturnsResultNode(t *testing.T) {
	rt := newTestRuntime(t)

	serviceName := "demo.msg.v1"
	node, nodeID, err := servicedesc.BuildDescriptorNodeV2(
		rt.identity.ID,
		rt.identity.PrivateKey,
		serviceName,
		[]servicedesc.Capability{{V: 1, JobType: "demo.msg.v1"}},
		map[string]uint64{"max_concurrency": 1, "max_payload_bytes": 1024},
		map[string]uint64{"cpu_cores": 4, "ram_mb": 4096},
	)
	if err != nil {
		t.Fatal(err)
	}
	nodeBytes, _, err := contentnode.EncodeWithCID(node)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.store.Put(nodeID, nodeBytes); err != nil {
		t.Fatal(err)
	}

	desc, err := servicedesc.Decode(node)
	if err != nil {
		t.Fatal(err)
	}
	nowMs := timeutil.NowUnixMs()
	head := netdb.ServiceHead{
		V:               1,
		ServiceID:       desc.ServiceID,
		OwnerIdentityID: rt.identity.ID,
		ServiceName:     serviceName,
		DescriptorCID:   nodeID,
		PublishedAtMs:   nowMs,
		ExpiresAtMs:     nowMs + int64((10*time.Minute)/time.Millisecond),
	}
	head, err = netdb.SignServiceHead(rt.identity.PrivateKey, head)
	if err != nil {
		t.Fatal(err)
	}
	headBytes, err := netdb.EncodeServiceHead(head)
	if err != nil {
		t.Fatal(err)
	}
	if status, code := rt.netdb.Store(headBytes, nowMs); status != "OK" {
		t.Fatalf("netdb store head: %s", code)
	}
	env := buildDirQueryRequestEnv(t, rt, nowMs, map[string]any{
		"service_name_prefix": "demo.",
		"requires":            []string{"demo.msg.v1"},
	}, 10)

	resps, err := rt.handleTaskV2(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(resps) == 0 {
		t.Fatal("expected responses")
	}

	respEnv, err := envelopev2.DecodeEnvelope(resps[len(resps)-1])
	if err != nil {
		t.Fatal(err)
	}
	if respEnv.Type != tasks.ResultType {
		t.Fatalf("expected result, got %s", respEnv.Type)
	}
	resPayload, err := tasks.DecodeResult(respEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if resPayload.ResultNodeID == "" {
		t.Fatal("missing result node id")
	}
	if b, err := rt.store.Get(resPayload.ResultNodeID); err == nil {
		if err := contentnode.VerifyBytes(b, resPayload.ResultNodeID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDirQueryV2RateLimit(t *testing.T) {
	cfg := config.Default()
	cfg.Observability.HealthAddr = freeAddr(t)
	cfg.Observability.MetricsAddr = freeAddr(t)
	cfg.Limits.DirQueryRateLimit = 1
	cfg.Limits.DirQueryRateWindowMs = int64((10 * time.Minute) / time.Millisecond)
	rt := New(cfg)
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = rt.Stop(context.Background())
	})

	nowMs := timeutil.NowUnixMs()
	env := buildDirQueryRequestEnv(t, rt, nowMs, map[string]any{
		"service_name_prefix": "demo.",
	}, 1)

	if _, err := rt.handleTaskV2(env); err != nil {
		t.Fatal(err)
	}

	env2 := buildDirQueryRequestEnv(t, rt, nowMs, map[string]any{
		"service_name_prefix": "demo.",
	}, 1)
	resps, err := rt.handleTaskV2(env2)
	if err != nil {
		t.Fatal(err)
	}
	if len(resps) == 0 {
		t.Fatal("expected responses")
	}
	respEnv, err := envelopev2.DecodeEnvelope(resps[len(resps)-1])
	if err != nil {
		t.Fatal(err)
	}
	if respEnv.Type != tasks.FailType {
		t.Fatalf("expected fail, got %s", respEnv.Type)
	}
	failPayload, err := tasks.DecodeFail(respEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if failPayload.ErrorCode != "ERR_DIR_RATE_LIMITED" {
		t.Fatalf("expected ERR_DIR_RATE_LIMITED, got %s", failPayload.ErrorCode)
	}
}

func buildDirQueryRequestEnv(t *testing.T, rt *Runtime, nowMs int64, query map[string]any, limit uint64) *envelopev2.Envelope {
	t.Helper()
	taskID, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	clientReqID, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	req := tasks.Request{
		V:               tasks.Version,
		TaskID:          taskID,
		ClientRequestID: clientReqID,
		JobType:         dirquery.JobType,
		Input: map[string]any{
			"v":     uint64(1),
			"query": query,
			"limit": limit,
		},
		TSMs: nowMs,
	}
	payload, err := tasks.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	env := &envelopev2.Envelope{
		V:       envelopev2.Version,
		MsgID:   taskID,
		Type:    tasks.RequestType,
		From:    envelopev2.From{IdentityID: rt.identity.ID},
		To:      envelopev2.To{ServiceID: "svc_dummy"},
		ReplyTo: &envelopev2.Reply{ServiceID: "svc_reply"},
		TSMs:    nowMs,
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	if err := env.Sign(rt.identity.PrivateKey); err != nil {
		t.Fatal(err)
	}
	return env
}
