package runtime

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestDegradedLowPeers(t *testing.T) {
	rt := newTestRuntime(t)
	if rt.net.State() != "online" && rt.net.State() != "degraded" {
		t.Fatalf("unexpected state %s", rt.net.State())
	}
	rt.net.AddDegradedReason("low_peers")
	if rt.net.State() != "degraded" {
		t.Fatalf("expected degraded, got %s", rt.net.State())
	}
}

func TestHealthEndpoint(t *testing.T) {
	rt := newTestRuntime(t)
	time.Sleep(50 * time.Millisecond)
	healthURL := buildHealthURL(rt.cfg.Observability.HealthAddr)
	// #nosec G107 -- health endpoint is loopback-only HTTP by design (test).
	resp, err := http.Get(healthURL)
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

func buildHealthURL(addr string) string {
	return (&url.URL{Scheme: "http", Host: addr, Path: "/healthz"}).String()
}
