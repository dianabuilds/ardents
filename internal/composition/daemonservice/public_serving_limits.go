package daemonservice

import (
	"strings"
	"time"

	"aim-chat/go-backend/internal/platform/ratelimiter"
)

func resolvePublicServingDegradeConfigFromEnv() publicServingDegradeConfig {
	return publicServingDegradeConfig{
		Enabled:                   envBoolWithFallback("AIM_PUBLIC_SERVING_AUTODEGRADE_ENABLED", true),
		OverloadWindow:            time.Duration(envBoundedIntWithFallback("AIM_PUBLIC_SERVING_OVERLOAD_WINDOW_SEC", 180, 5, 3600)) * time.Second,
		RecoveryWindow:            time.Duration(envBoundedIntWithFallback("AIM_PUBLIC_SERVING_RECOVERY_WINDOW_SEC", 600, 10, 7200)) * time.Second,
		RetryLoopLagThreshold:     time.Duration(envBoundedIntWithFallback("AIM_PUBLIC_SERVING_CPU_LAG_THRESHOLD_MS", 1500, 50, 60000)) * time.Millisecond,
		PendingQueueThreshold:     envBoundedIntWithFallback("AIM_PUBLIC_SERVING_CHAT_PENDING_THRESHOLD", 150, 1, 100000),
		RAMAllocThresholdMB:       envBoundedIntWithFallback("AIM_PUBLIC_SERVING_RAM_ALLOC_THRESHOLD_MB", 512, 64, 131072),
		DegradedServeFactorPct:    envBoundedIntWithFallback("AIM_PUBLIC_SERVING_DEGRADE_FACTOR_PCT", 50, 10, 90),
		DegradedConcurrentServes:  envBoundedIntWithFallback("AIM_PUBLIC_SERVING_DEGRADE_CONCURRENT", 1, 1, 128),
		DegradedPerPeerRequestsPM: envBoundedIntWithFallback("AIM_PUBLIC_SERVING_DEGRADE_PER_PEER_RPM", 30, 1, 10000),
	}
}

func (s *Service) configurePublicServingLimits(cfg blobNodePresetConfig) {
	if s == nil {
		return
	}
	if s.serveSoftLimiter != nil {
		s.serveSoftLimiter.SetLimitKBps(cfg.ServeBandwidthSoftKBps)
	}
	if s.serveLimiter != nil {
		s.serveLimiter.SetLimitKBps(cfg.ServeBandwidthHardKBps)
	}
	s.serveGuardMu.Lock()
	s.serveMaxConcurrent = normalizePositiveOrZero(cfg.ServeMaxConcurrent)
	s.servePerPeerPerMin = normalizePositiveOrZero(cfg.ServeRequestsPerMinPerPeer)
	if s.servePerPeerPerMin <= 0 {
		s.servePeerLimiter = nil
	} else {
		rps := float64(s.servePerPeerPerMin) / 60.0
		if rps <= 0 {
			s.servePeerLimiter = nil
		} else {
			burst := s.servePerPeerPerMin
			if burst < 1 {
				burst = 1
			}
			s.servePeerLimiter = ratelimiter.New(rps, burst, 10*time.Minute)
		}
	}
	s.serveGuardMu.Unlock()

	s.degradeMu.Lock()
	s.degradeState.BaseServeSoftKBps = cfg.ServeBandwidthSoftKBps
	s.degradeState.BaseServeHardKBps = cfg.ServeBandwidthHardKBps
	s.degradeState.BaseConcurrent = normalizePositiveOrZero(cfg.ServeMaxConcurrent)
	s.degradeState.BasePerPeerPerMin = normalizePositiveOrZero(cfg.ServeRequestsPerMinPerPeer)
	s.degradeMu.Unlock()
	s.configurePublicEphemeralCache(cfg)
}

func (s *Service) isPublicServingAllowed() bool {
	s.presetMu.RLock()
	enabled := s.nodePreset.PublicServingEnabled
	s.presetMu.RUnlock()
	return enabled
}

func (s *Service) isPublicStoreEnabled() bool {
	s.presetMu.RLock()
	enabled := s.nodePreset.PublicStoreEnabled
	s.presetMu.RUnlock()
	return enabled
}

func (s *Service) configurePublicEphemeralCache(cfg blobNodePresetConfig) {
	if s == nil || s.publicBlobCache == nil {
		return
	}
	s.publicBlobCache.Configure(cfg.PublicEphemeralCacheMaxMB, cfg.PublicEphemeralCacheTTLMin)
}

func (s *Service) purgePublicEphemeralCache(now time.Time) {
	if s == nil || s.publicBlobCache == nil {
		return
	}
	s.publicBlobCache.PurgeExpired(now)
}

func (s *Service) allowPublicServeRequest(requesterPeerID string) bool {
	requesterPeerID = strings.TrimSpace(requesterPeerID)
	if requesterPeerID == "" {
		requesterPeerID = "unknown"
	}
	s.serveGuardMu.Lock()
	limiter := s.servePeerLimiter
	s.serveGuardMu.Unlock()
	if limiter == nil {
		return true
	}
	return limiter.Allow(requesterPeerID, time.Now())
}

func (s *Service) acquirePublicServeSlot() (func(), bool) {
	s.serveGuardMu.Lock()
	defer s.serveGuardMu.Unlock()
	if s.serveMaxConcurrent > 0 && s.serveInFlight >= s.serveMaxConcurrent {
		return nil, false
	}
	s.serveInFlight++
	released := false
	return func() {
		s.serveGuardMu.Lock()
		defer s.serveGuardMu.Unlock()
		if released {
			return
		}
		released = true
		if s.serveInFlight > 0 {
			s.serveInFlight--
		}
	}, true
}

func (s *Service) markPublicServeSoftCapExceeded() {
	s.degradeMu.Lock()
	s.degradeState.SoftCapExceeded = true
	s.degradeMu.Unlock()
}

func normalizePositiveOrZero(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func (s *Service) currentPublicServingConcurrencyLimit() int {
	s.serveGuardMu.Lock()
	defer s.serveGuardMu.Unlock()
	return s.serveMaxConcurrent
}

func (s *Service) currentPublicServingPeerLimitPerMin() int {
	s.serveGuardMu.Lock()
	defer s.serveGuardMu.Unlock()
	return s.servePerPeerPerMin
}

func (s *Service) isPublicServingDegraded() bool {
	s.degradeMu.Lock()
	defer s.degradeMu.Unlock()
	return s.degradeState.Degraded
}
