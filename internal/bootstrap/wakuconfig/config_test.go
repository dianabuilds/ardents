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
		FailoverV1:          boolPtr(true),
		MinPeers:            4,
		StoreQueryFanout:    5,
		ReconnectInterval:   2 * time.Second,
		ReconnectBackoffMax: 45 * time.Second,
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
