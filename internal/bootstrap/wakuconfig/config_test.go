package wakuconfig

import (
	"testing"
	"time"

	"aim-chat/go-backend/internal/waku"
)

func boolPtr(v bool) *bool {
	return &v
}

func TestMergeIncludesFailoverFields(t *testing.T) {
	dst := waku.DefaultConfig()
	src := DaemonNetworkConfig{
		FailoverV1:                 boolPtr(true),
		MinPeers:                   4,
		StoreQueryFanout:           5,
		ReconnectInterval:          2 * time.Second,
		ReconnectBackoffMax:        45 * time.Second,
		ManifestRefreshInterval:    30 * time.Second,
		ManifestStaleWindow:        3 * time.Minute,
		ManifestRefreshTimeout:     3 * time.Second,
		ManifestBackoffBase:        2 * time.Second,
		ManifestBackoffMax:         40 * time.Second,
		ManifestBackoffFactor:      2.5,
		ManifestBackoffJitterRatio: 0.35,
	}

	Merge(&dst, src)

	if dst.MinPeers != 4 {
		t.Fatalf("expected minPeers=4, got %d", dst.MinPeers)
	}
	if dst.StoreQueryFanout != 5 {
		t.Fatalf("expected storeQueryFanout=5, got %d", dst.StoreQueryFanout)
	}
	if dst.ReconnectInterval != 2*time.Second {
		t.Fatalf("expected reconnectInterval=2s, got %s", dst.ReconnectInterval)
	}
	if dst.ReconnectBackoffMax != 45*time.Second {
		t.Fatalf("expected reconnectBackoffMax=45s, got %s", dst.ReconnectBackoffMax)
	}
	if dst.ManifestRefreshInterval != 30*time.Second {
		t.Fatalf("expected manifestRefreshInterval=30s, got %s", dst.ManifestRefreshInterval)
	}
	if dst.ManifestStaleWindow != 3*time.Minute {
		t.Fatalf("expected manifestStaleWindow=3m, got %s", dst.ManifestStaleWindow)
	}
	if dst.ManifestRefreshTimeout != 3*time.Second {
		t.Fatalf("expected manifestRefreshTimeout=3s, got %s", dst.ManifestRefreshTimeout)
	}
	if dst.ManifestBackoffBase != 2*time.Second {
		t.Fatalf("expected manifestBackoffBase=2s, got %s", dst.ManifestBackoffBase)
	}
	if dst.ManifestBackoffMax != 40*time.Second {
		t.Fatalf("expected manifestBackoffMax=40s, got %s", dst.ManifestBackoffMax)
	}
	if dst.ManifestBackoffFactor != 2.5 {
		t.Fatalf("expected manifestBackoffFactor=2.5, got %v", dst.ManifestBackoffFactor)
	}
	if dst.ManifestBackoffJitterRatio != 0.35 {
		t.Fatalf("expected manifestBackoffJitterRatio=0.35, got %v", dst.ManifestBackoffJitterRatio)
	}
	if !dst.FailoverV1 {
		t.Fatal("expected failoverV1=true after merge")
	}
}

func TestMergeDoesNotOverwriteBoolDefaultsWhenUnset(t *testing.T) {
	dst := waku.DefaultConfig()
	dst.EnableRelay = true
	dst.EnableStore = true
	dst.EnableFilter = true
	dst.EnableLightPush = true
	dst.FailoverV1 = true

	src := DaemonNetworkConfig{
		Transport: "go-waku",
	}

	Merge(&dst, src)

	if !dst.EnableRelay || !dst.EnableStore || !dst.EnableFilter || !dst.EnableLightPush || !dst.FailoverV1 {
		t.Fatal("unset bool fields must not overwrite existing defaults")
	}
}

func TestMergeAppliesExplicitBoolFalseAndTrue(t *testing.T) {
	dst := waku.DefaultConfig()
	dst.EnableRelay = true
	dst.EnableStore = true
	dst.EnableFilter = true
	dst.EnableLightPush = true
	dst.FailoverV1 = true

	src := DaemonNetworkConfig{
		EnableRelay:     boolPtr(false),
		EnableStore:     boolPtr(false),
		EnableFilter:    boolPtr(false),
		EnableLightPush: boolPtr(true),
		FailoverV1:      boolPtr(false),
	}

	Merge(&dst, src)

	if dst.EnableRelay {
		t.Fatal("expected enableRelay=false from explicit config")
	}
	if dst.EnableStore {
		t.Fatal("expected enableStore=false from explicit config")
	}
	if dst.EnableFilter {
		t.Fatal("expected enableFilter=false from explicit config")
	}
	if !dst.EnableLightPush {
		t.Fatal("expected enableLightPush=true from explicit config")
	}
	if dst.FailoverV1 {
		t.Fatal("expected failoverV1=false from explicit config")
	}
}

func TestApplyEnvOverridesCanDisableFailover(t *testing.T) {
	t.Setenv("AIM_NETWORK_FAILOVER_V1", "false")
	cfg := waku.DefaultConfig()
	if !cfg.FailoverV1 {
		t.Fatal("expected default failoverV1=true")
	}
	ApplyEnvOverrides(&cfg)
	if cfg.FailoverV1 {
		t.Fatal("expected failoverV1=false from env override")
	}
}

func TestApplyEnvOverridesCanEnableFailover(t *testing.T) {
	t.Setenv("AIM_NETWORK_FAILOVER_V1", "true")
	cfg := waku.DefaultConfig()
	cfg.FailoverV1 = false
	ApplyEnvOverrides(&cfg)
	if !cfg.FailoverV1 {
		t.Fatal("expected failoverV1=true from env override")
	}
}

func TestApplyEnvOverridesIgnoresInvalidValue(t *testing.T) {
	t.Setenv("AIM_NETWORK_FAILOVER_V1", "invalid")
	cfg := waku.DefaultConfig()
	cfg.FailoverV1 = false
	ApplyEnvOverrides(&cfg)
	if cfg.FailoverV1 {
		t.Fatal("invalid env value must not change failoverV1")
	}
}

func TestApplyEnvOverridesIncludesManifestRefreshPolicy(t *testing.T) {
	t.Setenv("AIM_MANIFEST_REFRESH_INTERVAL", "45s")
	t.Setenv("AIM_MANIFEST_STALE_WINDOW", "4m")
	t.Setenv("AIM_MANIFEST_REFRESH_TIMEOUT", "4s")
	t.Setenv("AIM_MANIFEST_BACKOFF_BASE", "1500ms")
	t.Setenv("AIM_MANIFEST_BACKOFF_MAX", "25s")
	t.Setenv("AIM_MANIFEST_BACKOFF_FACTOR", "2.25")
	t.Setenv("AIM_MANIFEST_BACKOFF_JITTER_RATIO", "0.4")

	cfg := waku.DefaultConfig()
	ApplyEnvOverrides(&cfg)

	if cfg.ManifestRefreshInterval != 45*time.Second {
		t.Fatalf("expected manifestRefreshInterval=45s, got %s", cfg.ManifestRefreshInterval)
	}
	if cfg.ManifestStaleWindow != 4*time.Minute {
		t.Fatalf("expected manifestStaleWindow=4m, got %s", cfg.ManifestStaleWindow)
	}
	if cfg.ManifestRefreshTimeout != 4*time.Second {
		t.Fatalf("expected manifestRefreshTimeout=4s, got %s", cfg.ManifestRefreshTimeout)
	}
	if cfg.ManifestBackoffBase != 1500*time.Millisecond {
		t.Fatalf("expected manifestBackoffBase=1500ms, got %s", cfg.ManifestBackoffBase)
	}
	if cfg.ManifestBackoffMax != 25*time.Second {
		t.Fatalf("expected manifestBackoffMax=25s, got %s", cfg.ManifestBackoffMax)
	}
	if cfg.ManifestBackoffFactor != 2.25 {
		t.Fatalf("expected manifestBackoffFactor=2.25, got %v", cfg.ManifestBackoffFactor)
	}
	if cfg.ManifestBackoffJitterRatio != 0.4 {
		t.Fatalf("expected manifestBackoffJitterRatio=0.4, got %v", cfg.ManifestBackoffJitterRatio)
	}
}
