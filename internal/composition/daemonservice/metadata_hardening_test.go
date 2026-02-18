package daemonservice

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"aim-chat/go-backend/internal/domains/contracts"
)

func TestMetadataHardeningAddsPaddingToBucket(t *testing.T) {
	h := &outboundMetadataHardening{
		enabled:      true,
		batchWindow:  50 * time.Millisecond,
		jitterMax:    30 * time.Millisecond,
		randomSource: rand.New(rand.NewSource(1)),
	}
	wire := contracts.WirePayload{
		Kind:  "plain",
		Plain: []byte("hello"),
	}
	hardened, delay, err := h.harden(wire)
	if err != nil {
		t.Fatalf("harden failed: %v", err)
	}
	if hardened.Padding == "" {
		t.Fatal("expected non-empty padding for non-latency payload")
	}
	raw, err := json.Marshal(hardened)
	if err != nil {
		t.Fatalf("marshal hardened payload: %v", err)
	}
	validBucket := false
	for _, bucket := range sizeBuckets {
		if len(raw) == bucket {
			validBucket = true
			break
		}
	}
	if !validBucket {
		t.Fatalf("payload size must match bucket, got=%d", len(raw))
	}
	if delay < h.batchWindow || delay > h.batchWindow+h.jitterMax {
		t.Fatalf("unexpected delay=%v", delay)
	}
}

func TestMetadataHardeningSkipsLatencyCriticalKinds(t *testing.T) {
	h := &outboundMetadataHardening{
		enabled:      true,
		batchWindow:  80 * time.Millisecond,
		jitterMax:    200 * time.Millisecond,
		randomSource: rand.New(rand.NewSource(1)),
	}
	wire := contracts.WirePayload{
		Kind: "receipt",
	}
	hardened, delay, err := h.harden(wire)
	if err != nil {
		t.Fatalf("harden failed: %v", err)
	}
	if hardened.Padding != "" {
		t.Fatal("latency-critical payload must not be padded")
	}
	if delay != 0 {
		t.Fatalf("latency-critical payload must not be delayed, got=%v", delay)
	}
}
