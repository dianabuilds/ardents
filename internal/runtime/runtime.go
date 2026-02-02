package runtime

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dianabuilds/ardents/internal/addressbook"
	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/delivery"
	"github.com/dianabuilds/ardents/internal/health"
	"github.com/dianabuilds/ardents/internal/metrics"
	netpkg "github.com/dianabuilds/ardents/internal/net"
	"github.com/dianabuilds/ardents/internal/netmgr"
	"github.com/dianabuilds/ardents/internal/observability"
	"github.com/dianabuilds/ardents/internal/providers"
	"github.com/dianabuilds/ardents/internal/services/serviceregistry"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/capabilities"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/storage"
	"github.com/dianabuilds/ardents/internal/transport/quic"
)

type Runtime struct {
	cfg               config.Config
	net               *netmgr.Manager
	dedup             *netpkg.Dedup
	bans              *netpkg.BanList
	quic              *quic.Server
	dial              *quic.Dialer
	peerID            string
	store             *storage.NodeStore
	identity          identity.Identity
	book              addressbook.Book
	log               *observability.Logger
	pcap              *observability.PcapWriter
	pcapPath          string
	tracker           *delivery.Tracker
	health            *health.Server
	peersConnected    uint64
	metrics           *metrics.Registry
	metricsServer     *metrics.Server
	providers         *providers.Registry
	services          *serviceregistry.Registry
	tasks             *TaskStore
	clockSkew         *clockSkewTracker
	powAbuse          *powAbuseTracker
	localCapabilities []string
	capsMu            sync.Mutex
	peerCaps          map[string][]byte
	transportKeys     quic.KeyMaterial
	relayForward      func(peerID string, envBytes []byte) error
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
	dirs, err := appdirs.Resolve("")
	if err != nil {
		dirs = appdirs.Dirs{
			ConfigDir: "config",
			DataDir:   "data",
			RunDir:    "run",
			StateDir:  "run",
		}
	}

	id, err := identity.LoadOrCreate(dirs.IdentityDir())
	if err != nil {
		id = identity.Identity{}
	}
	book, err := addressbook.LoadOrInit(dirs.AddressBookPath())
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
	keys, err := quic.LoadOrCreateKeyMaterial(dirs.KeysDir())
	if err != nil {
		keys = quic.KeyMaterial{}
	}
	pcapPath := dirs.PcapPath()
	pcap := observability.NewPcapWriter(cfg.Observability.PcapEnabled, pcapPath)
	logFile := cfg.Observability.LogFile
	if logFile != "" && !filepath.IsAbs(logFile) {
		logFile = filepath.Join(dirs.RunDir, logFile)
	}
	log := observability.NewWithOptions(cfg.Observability.LogFormat, logFile)
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
		store:             storage.NewNodeStore(1_048_576),
		identity:          id,
		book:              book,
		log:               log,
		pcap:              pcap,
		pcapPath:          pcapPath,
		tracker:           delivery.NewTracker(),
		metrics:           metrics.New(),
		providers:         providers.NewRegistry(),
		services:          serviceregistry.New(),
		tasks:             NewTaskStore(24 * time.Hour),
		clockSkew:         newClockSkewTracker(4),
		powAbuse:          newPowAbuseTracker(5),
		localCapabilities: []string{"node.fetch.v1"},
		peerCaps:          make(map[string][]byte),
		transportKeys:     keys,
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
	return string(r.net.State()), atomic.LoadUint64(&r.peersConnected)
}

func (r *Runtime) DedupSeen(msgID string) bool {
	return r.dedup.SeenWithTTL(msgID, 0)
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

func (r *Runtime) IdentityPrivateKey() ed25519.PrivateKey {
	return r.identity.PrivateKey
}

func (r *Runtime) checkClockSkew(nowMs int64) {
	_ = nowMs
}

func (r *Runtime) checkLowPeers() {
	if atomic.LoadUint64(&r.peersConnected) < 1 {
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

func (r *Runtime) observeHello(peerID string, remoteTSMs int64, digest []byte) {
	if r == nil || r.clockSkew == nil {
		return
	}
	now := timeutil.NowUnixMs()
	skewed := skewedNow(now, remoteTSMs)
	if _, reached := r.clockSkew.Observe(peerID, skewed); reached {
		r.net.AddDegradedReason("clock_skew")
		r.log.Event("warn", "net", "net.clock_skew", peerID, "", "clock_skew")
		r.log.Event("warn", "net", "net.degraded", "", "", "clock_skew")
	}
	if r.services != nil && r.capabilitiesChanged(peerID, digest) {
		removed := r.services.PurgeByPeer(peerID)
		r.log.Event("info", "service", "service.capabilities.changed", peerID, "", "")
		if removed > 0 {
			r.log.Event("info", "service", "service.descriptors.purged", peerID, "", "")
		}
	}
}

func (r *Runtime) observePeerConnected(peerID string) {
	atomic.AddUint64(&r.peersConnected, 1)
	if r.metrics != nil {
		r.metrics.IncNetInbound()
	}
	r.log.Event("info", "net", "peer.connected", peerID, "", "")
	r.checkLowPeers()
}

func (r *Runtime) observePeerDisconnected(peerID string) {
	if atomic.LoadUint64(&r.peersConnected) > 0 {
		atomic.AddUint64(&r.peersConnected, ^uint64(0))
	}
	if r.metrics != nil {
		r.metrics.DecNetInbound()
	}
	r.log.Event("info", "net", "peer.disconnected", peerID, "", "")
	r.checkLowPeers()
}

func (r *Runtime) capabilitiesDigest() []byte {
	digest, err := capabilities.Digest(r.localCapabilities)
	if err != nil {
		return nil
	}
	return digest
}

func (r *Runtime) capabilitiesChanged(peerID string, digest []byte) bool {
	if peerID == "" {
		return false
	}
	r.capsMu.Lock()
	defer r.capsMu.Unlock()
	prev, ok := r.peerCaps[peerID]
	if ok && bytesEqual(prev, digest) {
		return false
	}
	r.peerCaps[peerID] = append([]byte(nil), digest...)
	return ok
}

func bytesEqual(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
