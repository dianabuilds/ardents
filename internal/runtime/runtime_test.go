package runtime

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/config"
)

func TestDegradedLowPeers(t *testing.T) {
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
	if rt.net.State() != "online" && rt.net.State() != "degraded" {
		t.Fatalf("unexpected state %s", rt.net.State())
	}
	rt.net.AddDegradedReason("low_peers")
	if rt.net.State() != "degraded" {
		t.Fatalf("expected degraded, got %s", rt.net.State())
	}
}

func TestHealthEndpoint(t *testing.T) {
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
	time.Sleep(50 * time.Millisecond)
	resp, err := http.Get("http://" + rt.cfg.Observability.HealthAddr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Fatal(cerr)
		}
	}()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
