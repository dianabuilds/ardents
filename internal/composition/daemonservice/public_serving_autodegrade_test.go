package daemonservice

import (
	"testing"
	"time"
)

func TestPublicServingAutodegradeAndRecovery(t *testing.T) {
	t.Setenv("AIM_PUBLIC_SERVING_AUTODEGRADE_ENABLED", "true")
	t.Setenv("AIM_PUBLIC_SERVING_OVERLOAD_WINDOW_SEC", "5")
	t.Setenv("AIM_PUBLIC_SERVING_RECOVERY_WINDOW_SEC", "10")
	t.Setenv("AIM_PUBLIC_SERVING_CPU_LAG_THRESHOLD_MS", "50")
	t.Setenv("AIM_PUBLIC_SERVING_RAM_ALLOC_THRESHOLD_MB", "104857")
	t.Setenv("AIM_PUBLIC_SERVING_CHAT_PENDING_THRESHOLD", "100000")

	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	baseSoft := svc.serveSoftLimiter.LimitKBps()
	baseHard := svc.serveLimiter.LimitKBps()
	baseConcurrent := svc.currentPublicServingConcurrencyLimit()
	basePerPeer := svc.currentPublicServingPeerLimitPerMin()

	now := time.Now()
	svc.evaluatePublicServingAutodegrade(now, 100*time.Millisecond)
	if svc.isPublicServingDegraded() {
		t.Fatal("must not degrade before overload window is reached")
	}

	svc.evaluatePublicServingAutodegrade(now.Add(6*time.Second), 100*time.Millisecond)
	if !svc.isPublicServingDegraded() {
		t.Fatal("expected degraded state after sustained overload")
	}
	if svc.serveSoftLimiter.LimitKBps() >= baseSoft || svc.serveLimiter.LimitKBps() >= baseHard {
		t.Fatalf("expected lowered bandwidth limits in degraded mode: soft=%d hard=%d baseSoft=%d baseHard=%d",
			svc.serveSoftLimiter.LimitKBps(), svc.serveLimiter.LimitKBps(), baseSoft, baseHard)
	}
	if svc.currentPublicServingConcurrencyLimit() >= baseConcurrent || svc.currentPublicServingPeerLimitPerMin() >= basePerPeer {
		t.Fatalf("expected lowered serve guards in degraded mode: concurrent=%d perPeer=%d",
			svc.currentPublicServingConcurrencyLimit(), svc.currentPublicServingPeerLimitPerMin())
	}

	svc.evaluatePublicServingAutodegrade(now.Add(7*time.Second), 0)
	svc.evaluatePublicServingAutodegrade(now.Add(18*time.Second), 0)
	if svc.isPublicServingDegraded() {
		t.Fatal("expected recovery to restore normal serving limits")
	}
	if svc.serveSoftLimiter.LimitKBps() != baseSoft || svc.serveLimiter.LimitKBps() != baseHard {
		t.Fatalf("expected restored bandwidth limits after recovery: soft=%d hard=%d baseSoft=%d baseHard=%d",
			svc.serveSoftLimiter.LimitKBps(), svc.serveLimiter.LimitKBps(), baseSoft, baseHard)
	}
	if svc.currentPublicServingConcurrencyLimit() != baseConcurrent || svc.currentPublicServingPeerLimitPerMin() != basePerPeer {
		t.Fatalf("expected restored serve guards after recovery: concurrent=%d perPeer=%d baseConcurrent=%d basePerPeer=%d",
			svc.currentPublicServingConcurrencyLimit(), svc.currentPublicServingPeerLimitPerMin(), baseConcurrent, basePerPeer)
	}
}

func TestPublicServingConcurrentLimit(t *testing.T) {
	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	custom := buildBlobNodePresetConfig(blobNodePresetAssist)
	custom.ServeMaxConcurrent = 1
	custom.ServeRequestsPerMinPerPeer = 1000
	svc.configurePublicServingLimits(custom)

	releaseOne, ok := svc.acquirePublicServeSlot()
	if !ok {
		t.Fatal("first serve slot must be available")
	}
	defer releaseOne()
	if _, ok := svc.acquirePublicServeSlot(); ok {
		t.Fatal("second serve slot must be blocked when concurrent limit reached")
	}
}

func TestPublicServingPerPeerRateLimit(t *testing.T) {
	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	custom := buildBlobNodePresetConfig(blobNodePresetAssist)
	custom.ServeMaxConcurrent = 4
	custom.ServeRequestsPerMinPerPeer = 1
	svc.configurePublicServingLimits(custom)

	if !svc.allowPublicServeRequest("peer-A") {
		t.Fatal("first request for peer must pass")
	}
	if svc.allowPublicServeRequest("peer-A") {
		t.Fatal("second immediate request for same peer must be rate-limited")
	}
}
