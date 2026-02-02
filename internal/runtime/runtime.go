package runtime

import (
	"context"
	"time"

	"github.com/dianabuilds/ardents/internal/addressbook"
	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/delivery"
	"github.com/dianabuilds/ardents/internal/health"
	"github.com/dianabuilds/ardents/internal/metrics"
	netpkg "github.com/dianabuilds/ardents/internal/net"
	"github.com/dianabuilds/ardents/internal/netmgr"
	"github.com/dianabuilds/ardents/internal/observability"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/storage"
	"github.com/dianabuilds/ardents/internal/transport/quic"
)

type Runtime struct {
	cfg            config.Config
	net            *netmgr.Manager
	dedup          *netpkg.Dedup
	bans           *netpkg.BanList
	quic           *quic.Server
	dial           *quic.Dialer
	peerID         string
	store          *storage.NodeStore
	identity       identity.Identity
	book           addressbook.Book
	log            *observability.Logger
	tracker        *delivery.Tracker
	health         *health.Server
	peersConnected uint64
	metrics        *metrics.Registry
	metricsServer  *metrics.Server
}

func New(cfg config.Config) *Runtime {
	qs, err := quic.NewServer(cfg)
	if err != nil {
		qs = nil
	}
	dialer, err := quic.NewDialer(cfg)
	if err != nil {
		dialer = nil
	}
	id, err := identity.LoadOrCreate("")
	if err != nil {
		id = identity.Identity{}
	}
	book, err := addressbook.LoadOrInit("")
	if err != nil {
		book = addressbook.Book{}
	}
	if id.ID != "" {
		book.Entries = append(book.Entries, addressbook.Entry{
			Alias:       "self",
			TargetType:  "identity",
			TargetID:    id.ID,
			Source:      "self",
			Trust:       "trusted",
			CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		})
	}
	return &Runtime{
		cfg:   cfg,
		net:   netmgr.New(),
		dedup: netpkg.NewDedup(10*time.Minute, int(cfg.Limits.MaxInflightMsgs)),
		bans:  netpkg.NewBanList(),
		quic:  qs,
		dial:  dialer,
		peerID: func() string {
			if qs != nil {
				return qs.PeerID()
			}
			return ""
		}(),
		store:    storage.NewNodeStore(1_048_576),
		identity: id,
		book:     book,
		log:      observability.New(),
		tracker:  delivery.NewTracker(),
		metrics:  metrics.New(),
	}
}

func (r *Runtime) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := r.net.Transition(netmgr.StateStarting); err != nil {
		return err
	}
	r.seedPlaceholderNode()
	observability.EnforceRetention("run/pcap.jsonl", 24*time.Hour, 0)
	observability.EnforceRetention("run/log.jsonl", 7*24*time.Hour, 1<<30)
	if r.quic != nil {
		r.quic.SetEnvelopeHandler(r.handleEnvelope)
		if err := r.quic.Start(ctx); err != nil {
			r.net.AddDegradedReason("transport_errors")
			r.log.Event("warn", "net", "net.degraded", "", "", "transport_errors")
		}
	}
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
	r.dialBootstrap(ctx)
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
	return string(r.net.State()), r.peersConnected
}

func (r *Runtime) DedupSeen(msgID string) bool {
	return r.dedup.Seen(msgID)
}

func (r *Runtime) Ban(peerID string, window time.Duration) {
	r.bans.Ban(peerID, window)
}

func (r *Runtime) IsBanned(peerID string) bool {
	return r.bans.IsBanned(peerID)
}

func (r *Runtime) QUICAddr() string {
	if r.quic == nil {
		return ""
	}
	return r.quic.Addr()
}

func (r *Runtime) PeerID() string {
	return r.peerID
}

func (r *Runtime) IdentityID() string {
	return r.identity.ID
}

func (r *Runtime) checkClockSkew(nowMs int64) {
	_ = nowMs
	// placeholder: real skew tracking should be fed by handshake observations
	if false {
		r.net.AddDegradedReason("clock_skew")
		r.log.Event("warn", "net", "net.degraded", "", "", "clock_skew")
	}
}

func (r *Runtime) checkLowPeers() {
	if r.peersConnected < 1 {
		r.net.AddDegradedReason("low_peers")
		r.log.Event("warn", "net", "net.degraded", "", "", "low_peers")
	} else {
		r.net.ClearDegradedReason("low_peers")
	}
}

func (r *Runtime) seedPlaceholderNode() {
	if r.store == nil {
		return
	}
	if err := r.store.Put("cidv1-placeholder", []byte{0x01, 0x02, 0x03}); err != nil {
		return
	}
}

func (r *Runtime) dialBootstrap(ctx context.Context) {
	if r.dial == nil {
		return
	}
	for _, bp := range r.cfg.BootstrapPeers {
		for _, addr := range bp.Addrs {
			if err := r.dial.DialWithRetry(ctx, addr, bp.PeerID); err != nil {
				r.log.Event("warn", "net", "net.bootstrap_failed", bp.PeerID, "", "ERR_BOOTSTRAP_DIAL")
			}
		}
	}
}
