package metrics

import (
	"bufio"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAckLatencyBuckets(t *testing.T) {
	reg := New()
	reg.ObserveAckLatency(10)
	reg.ObserveAckLatency(60)
	reg.ObserveAckLatency(250)
	reg.ObserveAckLatency(2001)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	got := parseMetrics(rr.Body.String())
	if got["ack_latency_ms_count"] != "4" {
		t.Fatalf("expected count 4, got %s", got["ack_latency_ms_count"])
	}
	if got["ack_latency_ms_bucket{le=\"50\"}"] != "1" {
		t.Fatalf("expected le=50 bucket 1, got %s", got["ack_latency_ms_bucket{le=\"50\"}"])
	}
	if got["ack_latency_ms_bucket{le=\"100\"}"] != "2" {
		t.Fatalf("expected le=100 bucket 2, got %s", got["ack_latency_ms_bucket{le=\"100\"}"])
	}
	if got["ack_latency_ms_bucket{le=\"250\"}"] != "3" {
		t.Fatalf("expected le=250 bucket 3, got %s", got["ack_latency_ms_bucket{le=\"250\"}"])
	}
	if got["ack_latency_ms_bucket{le=\"2000\"}"] != "3" {
		t.Fatalf("expected le=2000 bucket 3, got %s", got["ack_latency_ms_bucket{le=\"2000\"}"])
	}
	if got["ack_latency_ms_bucket{le=\"+Inf\"}"] != "4" {
		t.Fatalf("expected +Inf bucket 4, got %s", got["ack_latency_ms_bucket{le=\"+Inf\"}"])
	}
}

func parseMetrics(body string) map[string]string {
	out := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		out[parts[0]] = parts[1]
	}
	return out
}
