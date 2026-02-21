package waku

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

func TestNodeLifecycle(t *testing.T) {
	n := NewNode(DefaultConfig())
	initial := n.Status()
	if initial.State != StateDisconnected {
		t.Fatalf("expected disconnected initially, got %s", initial.State)
	}

	if err := n.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	started := n.Status()
	if started.State != StateConnected {
		t.Fatalf("expected connected after start, got %s", started.State)
	}
	if started.PeerCount <= 0 {
		t.Fatalf("expected peer count > 0, got %d", started.PeerCount)
	}

	if err := n.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	stopped := n.Status()
	if stopped.State != StateDisconnected {
		t.Fatalf("expected disconnected after stop, got %s", stopped.State)
	}
}

func TestNodeLifecycleGoWaku(t *testing.T) {
	if os.Getenv("AIM_RUN_REAL_WAKU_TESTS") != "true" {
		t.Skip("set AIM_RUN_REAL_WAKU_TESTS=true to run go-waku lifecycle test")
	}
	if newGoWakuBackend() == nil {
		t.Skip("go-waku backend is not enabled in this build")
	}

	cfg := DefaultConfig()
	cfg.Transport = TransportGoWaku
	cfg.Port = 0
	cfg.BootstrapNodes = nil

	n := NewNode(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := n.Start(ctx); err != nil {
		t.Fatalf("go-waku start failed: %v", err)
	}
	started := n.Status()
	if started.State != StateConnected && started.State != StateDegraded {
		t.Fatalf("expected connected/degraded after go-waku start, got %s", started.State)
	}
	if err := n.Stop(context.Background()); err != nil {
		t.Fatalf("go-waku stop failed: %v", err)
	}
}

func TestNodeRuntimeStateTransitionsByPeerCount(t *testing.T) {
	prevInterval := runtimeStatusPollInterval
	runtimeStatusPollInterval = 20 * time.Millisecond
	defer func() { runtimeStatusPollInterval = prevInterval }()

	backend := &fakeGoWakuBackend{peerCount: 1}
	n := NewNode(Config{Transport: TransportGoWaku})
	n.mu.Lock()
	n.gw = backend
	n.status.State = StateConnected
	n.status.PeerCount = 1
	n.status.LastSync = time.Now()
	n.mu.Unlock()
	n.startRuntimeMonitor()
	defer n.stopRuntimeMonitor()

	waitForState(t, n, StateConnected, 300*time.Millisecond)
	backend.setPeerCount(0)
	waitForState(t, n, StateDegraded, 500*time.Millisecond)
	backend.setPeerCount(2)
	waitForState(t, n, StateConnected, 500*time.Millisecond)
}

func TestNormalizeConfigAppliesSafeDefaults(t *testing.T) {
	cfg := normalizeConfig(Config{
		Transport:                  "",
		MinPeers:                   -1,
		StoreQueryFanout:           0,
		ReconnectInterval:          0,
		ReconnectBackoffMax:        10 * time.Millisecond,
		ManifestRefreshInterval:    0,
		ManifestStaleWindow:        0,
		ManifestRefreshTimeout:     0,
		ManifestBackoffBase:        0,
		ManifestBackoffMax:         100 * time.Millisecond,
		ManifestBackoffFactor:      0.5,
		ManifestBackoffJitterRatio: 2.0,
	})

	if cfg.Transport == "" {
		t.Fatal("transport must be defaulted")
	}
	if cfg.MinPeers != 0 {
		t.Fatalf("expected negative minPeers to clamp to 0, got %d", cfg.MinPeers)
	}
	if cfg.StoreQueryFanout <= 0 {
		t.Fatalf("storeQueryFanout must be > 0, got %d", cfg.StoreQueryFanout)
	}
	if cfg.ReconnectInterval <= 0 {
		t.Fatalf("reconnectInterval must be > 0, got %s", cfg.ReconnectInterval)
	}
	if cfg.ReconnectBackoffMax < cfg.ReconnectInterval {
		t.Fatalf("reconnectBackoffMax must be >= reconnectInterval, got max=%s interval=%s", cfg.ReconnectBackoffMax, cfg.ReconnectInterval)
	}
	if cfg.ManifestRefreshInterval <= 0 {
		t.Fatalf("manifestRefreshInterval must be > 0, got %s", cfg.ManifestRefreshInterval)
	}
	if cfg.ManifestStaleWindow <= 0 {
		t.Fatalf("manifestStaleWindow must be > 0, got %s", cfg.ManifestStaleWindow)
	}
	if cfg.ManifestRefreshTimeout <= 0 {
		t.Fatalf("manifestRefreshTimeout must be > 0, got %s", cfg.ManifestRefreshTimeout)
	}
	if cfg.ManifestBackoffBase <= 0 {
		t.Fatalf("manifestBackoffBase must be > 0, got %s", cfg.ManifestBackoffBase)
	}
	if cfg.ManifestBackoffMax < cfg.ManifestBackoffBase {
		t.Fatalf("manifestBackoffMax must be >= manifestBackoffBase, got max=%s base=%s", cfg.ManifestBackoffMax, cfg.ManifestBackoffBase)
	}
	if cfg.ManifestBackoffFactor < 1 {
		t.Fatalf("manifestBackoffFactor must be >= 1, got %v", cfg.ManifestBackoffFactor)
	}
	if cfg.ManifestBackoffJitterRatio > 1 || cfg.ManifestBackoffJitterRatio < 0 {
		t.Fatalf("manifestBackoffJitterRatio must be in [0..1], got %v", cfg.ManifestBackoffJitterRatio)
	}
}

func TestStartupStateFromPeerCount(t *testing.T) {
	cfg := Config{MinPeers: 2}
	if got := startupStateFromPeerCount(2, cfg); got != StateConnected {
		t.Fatalf("expected connected, got %s", got)
	}
	if got := startupStateFromPeerCount(0, cfg); got != StateDegraded {
		t.Fatalf("expected degraded, got %s", got)
	}
}

func TestStartupPeerTarget(t *testing.T) {
	if got := startupPeerTarget(Config{}); got != 1 {
		t.Fatalf("expected default startup target=1, got %d", got)
	}
	if got := startupPeerTarget(Config{MinPeers: 3, BootstrapNodes: []string{"a", "b"}}); got != 2 {
		t.Fatalf("expected target capped by bootstrap size to 2, got %d", got)
	}
}

func TestWaitForStartupPeerCountTimeoutReturnsDegradedCount(t *testing.T) {
	backend := &fakeGoWakuBackend{peerCount: 0}
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	cfg := Config{
		MinPeers:            2,
		ReconnectInterval:   50 * time.Millisecond,
		ReconnectBackoffMax: 200 * time.Millisecond,
	}
	got, err := waitForStartupPeerCount(ctx, backend, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected peer count=0 after timeout, got %d", got)
	}
}

func waitForState(t *testing.T, n *Node, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if n.Status().State == expected {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for state=%s, got=%s", expected, n.Status().State)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type fakeGoWakuBackend struct {
	mu        sync.RWMutex
	peerCount int
}

func (f *fakeGoWakuBackend) Start(_ context.Context, _ Config) error { return nil }
func (f *fakeGoWakuBackend) Stop()                                   {}
func (f *fakeGoWakuBackend) NetworkMetrics() map[string]int          { return map[string]int{} }
func (f *fakeGoWakuBackend) ApplyConfig(_ Config)                    {}
func (f *fakeGoWakuBackend) SetIdentity(_ string)                    {}
func (f *fakeGoWakuBackend) ListenAddresses() []string               { return nil }
func (f *fakeGoWakuBackend) SubscribePrivate(_ func(PrivateMessage)) error {
	return nil
}
func (f *fakeGoWakuBackend) PublishPrivate(_ context.Context, _ PrivateMessage) error {
	return nil
}
func (f *fakeGoWakuBackend) FetchPrivateSince(_ context.Context, _ string, _ time.Time, _ int) ([]PrivateMessage, error) {
	return nil, nil
}
func (f *fakeGoWakuBackend) PeerCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.peerCount
}
func (f *fakeGoWakuBackend) setPeerCount(v int) {
	f.mu.Lock()
	f.peerCount = v
	f.mu.Unlock()
}
