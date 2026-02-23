package daemonservice

import (
	"fmt"
	"runtime"
	"time"

	"aim-chat/go-backend/internal/platform/ratelimiter"
)

func (s *Service) evaluatePublicServingAutodegrade(now time.Time, retryLoopLag time.Duration) {
	if s == nil || !s.degradeCfg.Enabled {
		return
	}
	overloaded, reason := s.isPublicServingOverloaded(retryLoopLag)

	s.degradeMu.Lock()
	defer s.degradeMu.Unlock()

	state := s.degradeState
	if overloaded {
		if state.OverloadSince.IsZero() {
			state.OverloadSince = now
		}
		state.StableSince = time.Time{}
		state.LastReason = reason
		if !state.Degraded && now.Sub(state.OverloadSince) >= s.degradeCfg.OverloadWindow {
			s.applyPublicServingDegradedLimitsLocked(&state, now)
		}
	} else {
		state.OverloadSince = time.Time{}
		if state.StableSince.IsZero() {
			state.StableSince = now
		}
		if state.Degraded && now.Sub(state.StableSince) >= s.degradeCfg.RecoveryWindow {
			s.restorePublicServingLimitsLocked(&state)
		}
	}
	state.SoftCapExceeded = false
	s.degradeState = state
}

func (s *Service) isPublicServingOverloaded(retryLoopLag time.Duration) (bool, string) {
	if !s.isPublicServingAllowed() {
		return false, ""
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	allocMB := int(mem.Alloc / (1024 * 1024))
	if s.degradeCfg.RAMAllocThresholdMB > 0 && allocMB >= s.degradeCfg.RAMAllocThresholdMB {
		return true, fmt.Sprintf("ram_alloc_mb=%d", allocMB)
	}
	if s.degradeCfg.PendingQueueThreshold > 0 && s.messageStore.PendingCount() >= s.degradeCfg.PendingQueueThreshold {
		return true, fmt.Sprintf("chat_pending=%d", s.messageStore.PendingCount())
	}
	if retryLoopLag >= s.degradeCfg.RetryLoopLagThreshold {
		return true, fmt.Sprintf("retry_loop_lag_ms=%d", retryLoopLag.Milliseconds())
	}
	s.degradeMu.Lock()
	softCapExceeded := s.degradeState.SoftCapExceeded
	s.degradeMu.Unlock()
	if softCapExceeded {
		return true, "serve_soft_cap_exceeded"
	}
	return false, ""
}

func (s *Service) applyPublicServingDegradedLimitsLocked(state *publicServingDegradeState, now time.Time) {
	if state.Degraded {
		return
	}
	soft := state.BaseServeSoftKBps * s.degradeCfg.DegradedServeFactorPct / 100
	hard := state.BaseServeHardKBps * s.degradeCfg.DegradedServeFactorPct / 100
	if soft <= 0 {
		soft = 1
	}
	if hard < soft {
		hard = soft
	}
	if s.serveSoftLimiter != nil {
		s.serveSoftLimiter.SetLimitKBps(soft)
	}
	if s.serveLimiter != nil {
		s.serveLimiter.SetLimitKBps(hard)
	}
	s.serveGuardMu.Lock()
	s.serveMaxConcurrent = s.degradeCfg.DegradedConcurrentServes
	s.servePerPeerPerMin = s.degradeCfg.DegradedPerPeerRequestsPM
	rps := float64(s.degradeCfg.DegradedPerPeerRequestsPM) / 60.0
	s.servePeerLimiter = nil
	if rps > 0 {
		s.servePeerLimiter = ratelimiter.New(rps, s.degradeCfg.DegradedPerPeerRequestsPM, 10*time.Minute)
	}
	s.serveGuardMu.Unlock()
	state.Degraded = true
	state.DegradedAppliedAt = now
}

func (s *Service) restorePublicServingLimitsLocked(state *publicServingDegradeState) {
	if !state.Degraded {
		return
	}
	if s.serveSoftLimiter != nil {
		s.serveSoftLimiter.SetLimitKBps(state.BaseServeSoftKBps)
	}
	if s.serveLimiter != nil {
		s.serveLimiter.SetLimitKBps(state.BaseServeHardKBps)
	}
	s.serveGuardMu.Lock()
	s.serveMaxConcurrent = state.BaseConcurrent
	s.servePerPeerPerMin = state.BasePerPeerPerMin
	s.servePeerLimiter = nil
	if state.BasePerPeerPerMin > 0 {
		rps := float64(state.BasePerPeerPerMin) / 60.0
		if rps > 0 {
			s.servePeerLimiter = ratelimiter.New(rps, state.BasePerPeerPerMin, 10*time.Minute)
		}
	}
	s.serveGuardMu.Unlock()
	state.Degraded = false
	state.LastReason = ""
	state.DegradedAppliedAt = time.Time{}
}
