package runtime

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dianabuilds/ardents/internal/core/app/netmgr"
	"github.com/dianabuilds/ardents/internal/core/infra/metrics"
)

func hasReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

func getMetricLine(t *testing.T, reg *metrics.Registry, prefix string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	out := rr.Body.String()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func TestCheckClockSkew_OutOfRangeMarksDegradedAndIncrementsMetric(t *testing.T) {
	r := &Runtime{
		net:     netmgr.New(),
		metrics: metrics.New(),
	}

	r.checkClockSkew(0)

	if !hasReason(r.net.Reasons(), "clock_invalid") {
		t.Fatalf("expected degraded reason clock_invalid")
	}
	line := getMetricLine(t, r.metrics, "clock_invalid_total ")
	if line != "clock_invalid_total 1" {
		t.Fatalf("expected clock_invalid_total 1, got %q", line)
	}
}

func TestCheckClockSkew_BackwardsJumpMarksDegraded(t *testing.T) {
	r := &Runtime{net: netmgr.New()}

	r.checkClockSkew(1700000000000)          // valid baseline
	r.checkClockSkew(1700000000000 - 20_000) // backwards jump > 10s

	if !hasReason(r.net.Reasons(), "clock_invalid") {
		t.Fatalf("expected degraded reason clock_invalid on backwards jump")
	}
}

func TestCheckClockSkew_RecoveryClearsDegraded(t *testing.T) {
	r := &Runtime{net: netmgr.New()}

	r.checkClockSkew(0) // invalid
	if !hasReason(r.net.Reasons(), "clock_invalid") {
		t.Fatalf("expected degraded reason clock_invalid")
	}

	r.checkClockSkew(1700000000000) // valid again
	if hasReason(r.net.Reasons(), "clock_invalid") {
		t.Fatalf("expected clock_invalid to be cleared after recovery")
	}
}
