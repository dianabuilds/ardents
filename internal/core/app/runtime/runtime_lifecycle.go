package runtime

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netmgr"
	"github.com/dianabuilds/ardents/internal/core/infra/metrics"
	"github.com/dianabuilds/ardents/internal/core/infra/observability"
	"github.com/dianabuilds/ardents/internal/core/transport/health"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func (r *Runtime) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := r.net.Transition(netmgr.StateStarting); err != nil {
		return err
	}
	r.seedPlaceholderNode()
	observability.EnforceRetention(r.pcapPath, 24*time.Hour, 0)
	if r.cfg.Observability.LogFile != "" {
		logFile := r.cfg.Observability.LogFile
		if !filepath.IsAbs(logFile) {
			dirs, err := appdirs.Resolve("")
			if err == nil {
				logFile = filepath.Join(dirs.RunDir, logFile)
			} else {
				logFile = filepath.Join("run", logFile)
			}
		}
		observability.EnforceRetention(logFile, 7*24*time.Hour, 1<<30)
	}
	if r.pcap != nil && r.pcap.Enabled() {
		r.log.Event("warn", "pcap", "pcap.enabled", "", "", "")
	}
	if r.quic != nil {
		r.quic.SetCapabilitiesDigest(r.capabilitiesDigest())
		r.quic.SetHelloObserverWithDigest(r.observeHello)
		r.quic.SetPeerObserver(r.observePeerConnected, r.observePeerDisconnected)
		r.quic.SetEnvelopeHandler(r.handleEnvelope)
		if err := r.quic.Start(ctx); err != nil {
			r.net.AddDegradedReason("transport_errors")
			r.log.Event("warn", "net", "net.degraded", "", "", "transport_errors")
		}
	}
	r.publishRouterInfo()
	r.startRouterInfoTicker(ctx)
	r.checkClockSkew(timeutil.NowUnixMs())
	r.checkLowPeers()
	if r.cfg.Observability.HealthAddr != "" {
		h, err := health.Start(ctx, r.cfg.Observability.HealthAddr, r)
		if err != nil {
			r.log.Event("warn", "health", "health.start_failed", "", "", "ERR_HEALTH_START")
		} else {
			r.health = h
		}
	}
	if r.cfg.Observability.MetricsAddr != "" {
		r.metricsServer = metrics.Start(r.cfg.Observability.MetricsAddr, r.metrics)
	}
	if r.dial != nil {
		r.dial.SetCapabilitiesDigest(r.capabilitiesDigest())
		r.dial.SetHelloObserverWithDigest(r.observeHello)
	}
	r.applyReseed(ctx)
	r.dialBootstrap(ctx)
	r.startTunnelManager(ctx)
	r.startServicePublishTicker(ctx)
	if err := r.net.Transition(netmgr.StateOnline); err != nil {
		return err
	}
	_ = ctx
	return nil
}

func (r *Runtime) Stop(ctx context.Context) error {
	if r.quic != nil {
		if err := r.quic.Stop(ctx); err != nil {
			return err
		}
	}
	if r.health != nil {
		if err := r.health.Stop(ctx); err != nil {
			return err
		}
	}
	if r.metricsServer != nil {
		r.metricsServer.Stop()
	}
	if err := r.net.Transition(netmgr.StateStopping); err != nil {
		return err
	}
	if err := r.net.Transition(netmgr.StateStopped); err != nil {
		return err
	}
	_ = ctx
	return nil
}

func (r *Runtime) NetState() netmgr.State {
	return r.net.State()
}

func (r *Runtime) NetReasons() []string {
	return r.net.Reasons()
}

func (r *Runtime) Status() (state string, peersConnected uint64) {
	return string(r.net.State()), atomic.LoadUint64(&r.peersConnected)
}
