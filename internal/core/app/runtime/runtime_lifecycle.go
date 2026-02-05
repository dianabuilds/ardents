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
	r.enforceRetention()
	if r.pcap != nil && r.pcap.Enabled() {
		r.log.Event("warn", "pcap", "pcap.enabled", "", "", "")
	}
	r.startIPCIfEnabled()
	r.startQUIC(ctx)
	r.afterTransportStart(ctx)
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
	if r.ipc != nil {
		r.ipc.stop()
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
	return mapHealthState(r.net.State()), atomic.LoadUint64(&r.peersConnected)
}

func mapHealthState(state netmgr.State) string {
	switch state {
	case netmgr.StateOnline:
		return "ok"
	case netmgr.StateStopped, netmgr.StateStopping:
		return "stopped"
	default:
		return "degraded"
	}
}

func (r *Runtime) enforceRetention() {
	observability.EnforceRetention(r.pcapPath, 24*time.Hour, 0)
	logFile := r.cfg.Observability.LogFile
	if logFile == "" {
		return
	}
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

func (r *Runtime) startIPCIfEnabled() {
	if !r.cfg.Integration.Enabled {
		return
	}
	ipc, err := startIPC(r)
	if err != nil {
		if err.Error() == "ERR_GATEWAY_UNAUTHORIZED" {
			r.log.Event("warn", "ipc", "ipc.start_failed", "", "", "owner-only ACL required")
			return
		}
		r.log.Event("warn", "ipc", "ipc.start_failed", "", "", err.Error())
		return
	}
	r.ipc = ipc
}

func (r *Runtime) startQUIC(ctx context.Context) {
	if r.quic == nil {
		return
	}
	r.quic.SetCapabilitiesDigest(r.capabilitiesDigest())
	r.quic.SetHelloObserverWithDigest(r.observeHello)
	r.quic.SetHandshakeHintObserver(r.observeHandshakeHint)
	r.quic.SetPeerObserver(r.observePeerConnected, r.observePeerDisconnected)
	r.quic.SetHandshakeErrorObserver(r.observeHandshakeError)
	r.quic.SetConnErrorObserver(r.observeConnError)
	r.quic.SetPeerBanChecker(r.IsBanned)
	r.quic.SetEnvelopeHandler(r.handleEnvelope)
	if err := r.quic.Start(ctx); err != nil {
		r.net.AddDegradedReason("transport_errors")
		r.log.Event("warn", "net", "net.degraded", "", "", "transport_errors")
	}
}

func (r *Runtime) afterTransportStart(ctx context.Context) {
	r.checkClockSkew(timeutil.NowUnixMs())
	r.startClockSkewTicker(ctx)
	r.checkLowPeers()
	r.startHealth(ctx)
	r.startMetrics()
	r.initDialer()
	r.publishRouterInfo()
	r.startRouterInfoTicker(ctx)
	r.applyReseed(ctx)
	r.startTunnelManager(ctx)
	r.startServicePublishTicker(ctx)
	r.dialBootstrap(ctx)
}

func (r *Runtime) startHealth(ctx context.Context) {
	if r.cfg.Observability.HealthAddr == "" {
		return
	}
	h, err := health.Start(ctx, r.cfg.Observability.HealthAddr, r)
	if err != nil {
		r.log.Event("warn", "health", "health.start_failed", "", "", "ERR_HEALTH_START")
		return
	}
	r.health = h
}

func (r *Runtime) startMetrics() {
	if r.cfg.Observability.MetricsAddr == "" {
		return
	}
	r.metricsServer = metrics.Start(r.cfg.Observability.MetricsAddr, r.metrics)
}

func (r *Runtime) initDialer() {
	if r.dial == nil {
		return
	}
	r.dial.SetCapabilitiesDigest(r.capabilitiesDigest())
	r.dial.SetHelloObserverWithDigest(r.observeHello)
	r.dial.SetHandshakeHintObserver(r.observeHandshakeHint)
}
