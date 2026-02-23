package bootstrapmanager

import (
	"aim-chat/go-backend/internal/bootstrap/manifestruntime"
	"aim-chat/go-backend/internal/waku"
	"context"
	"strings"
	"time"
)

type Refresher struct {
	manager    *Manager
	cfg        *waku.Config
	controller *manifestruntime.Controller
	onApplied  func(waku.Config)
}

func NewRefresher(manager *Manager, cfg *waku.Config, onApplied func(waku.Config)) *Refresher {
	if cfg == nil {
		return nil
	}
	policyCfg := manifestruntime.Config{
		RefreshInterval:      cfg.ManifestRefreshInterval,
		StaleRefreshInterval: minDuration(cfg.ManifestRefreshInterval, 15*time.Second),
		SlowPollingInterval:  60 * time.Second,
		StaleWindow:          cfg.ManifestStaleWindow,
		BackoffBase:          cfg.ManifestBackoffBase,
		BackoffMax:           cfg.ManifestBackoffMax,
		BackoffFactor:        cfg.ManifestBackoffFactor,
		BackoffJitterRatio:   cfg.ManifestBackoffJitterRatio,
	}
	return &Refresher{
		manager:    manager,
		cfg:        cfg,
		controller: manifestruntime.NewController(policyCfg, nil),
		onApplied:  onApplied,
	}
}

func (r *Refresher) Run(ctx context.Context) {
	if r == nil || r.manager == nil || r.cfg == nil {
		return
	}
	delay := time.Duration(0)
	for {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		} else {
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
		decision := r.step(time.Now().UTC())
		delay = decision.NextDelay
	}
}

func (r *Refresher) step(now time.Time) manifestruntime.Decision {
	load := r.manager.LoadBootstrapSet()
	outcome := manifestruntime.AttemptOutcome{
		ManifestAccepted: false,
		ErrorKind:        manifestruntime.ErrorNonRecoverable,
	}

	if load.OK && load.Set != nil {
		apply := r.manager.ApplyBootstrapSet(r.cfg, *load.Set)
		if apply.Applied {
			if r.onApplied != nil {
				r.onApplied(*r.cfg)
			}
			switch load.Set.Source {
			case SourceManifest:
				outcome.ManifestAccepted = true
				if load.Set.ManifestMeta != nil {
					outcome.ManifestExpiresAt = load.Set.ManifestMeta.ExpiresAt
				}
				outcome.ErrorKind = manifestruntime.ErrorNone
			case SourceCache:
				outcome.CacheUsable = true
				outcome.ErrorKind = classifyError(r.manager.LastReason())
			case SourceBaked:
				outcome.BakedUsable = true
				outcome.ErrorKind = classifyError(r.manager.LastReason())
			}
		}
	}
	return r.controller.Decide(now, outcome)
}

func classifyError(reason string) manifestruntime.ErrorKind {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return manifestruntime.ErrorRecoverable
	}
	if strings.Contains(reason, "timeout") || strings.Contains(reason, "network") || strings.Contains(reason, "temporar") {
		return manifestruntime.ErrorRecoverable
	}
	return manifestruntime.ErrorNonRecoverable
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}
