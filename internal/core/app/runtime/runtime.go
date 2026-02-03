package runtime

import (
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/netmgr"
	"github.com/dianabuilds/ardents/internal/core/app/services/serviceregistry"
	"github.com/dianabuilds/ardents/internal/core/domain/delivery"
	netpkg "github.com/dianabuilds/ardents/internal/core/domain/net"
	"github.com/dianabuilds/ardents/internal/core/domain/providers"
	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/infra/metrics"
	"github.com/dianabuilds/ardents/internal/core/infra/observability"
	"github.com/dianabuilds/ardents/internal/core/infra/reseed"
	"github.com/dianabuilds/ardents/internal/core/infra/storage"
	"github.com/dianabuilds/ardents/internal/core/transport/health"
	"github.com/dianabuilds/ardents/internal/core/transport/quic"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/capabilities"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/onionkey"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
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
	netdb             *netdb.DB
	clockSkew         *clockSkewTracker
	powAbuse          *powAbuseTracker
	localCapabilities []string
	capsMu            sync.Mutex
	peerCaps          map[string][]byte
	tunnelMu          sync.Mutex
	tunnels           map[string]*tunnelSession
	tunnelMgrMu       sync.Mutex
	outboundTunnels   []*tunnelPath
	inboundTunnels    []*tunnelPath
	tunnelMgrCancel   context.CancelFunc
	localSvcMu        sync.Mutex
	localServices     map[string]localServiceInfo
	transportKeys     quic.KeyMaterial
	onionKey          onionkey.Keypair
	relayForward      func(peerID string, envBytes []byte) error
	bootstrapPeers    []config.BootstrapPeer
	reseedParams      reseed.Params
	ipc               *ipcServer
}

func New(cfg config.Config) *Runtime {
	qs := newQUICServer(cfg)
	dialer := newDialer(cfg)
	dirs := resolveDirs()
	id, book := loadIdentityAndBook(dirs)
	keys, onion := loadLocalKeys(dirs)
	log, pcap, pcapPath := initObservability(cfg, dirs)
	peerID := ""
	if qs != nil {
		peerID = qs.PeerID()
	}
	return &Runtime{
		cfg:               cfg,
		net:               netmgr.New(),
		dedup:             netpkg.NewDedup(10*time.Minute, int(cfg.Limits.MaxInflightMsgs)),
		bans:              netpkg.NewBanList(),
		quic:              qs,
		dial:              dialer,
		peerID:            peerID,
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
		netdb:             netdb.New(netdb.DefaultRecordMaxTTLMs, netdb.DefaultK),
		clockSkew:         newClockSkewTracker(4),
		powAbuse:          newPowAbuseTracker(5),
		localCapabilities: []string{"node.fetch.v1"},
		peerCaps:          make(map[string][]byte),
		tunnels:           make(map[string]*tunnelSession),
		localServices:     make(map[string]localServiceInfo),
		transportKeys:     keys,
		onionKey:          onion,
	}
}

func fallbackDirs() appdirs.Dirs {
	base := filepath.Join(os.TempDir(), "ardents")
	return appdirs.Dirs{
		Home:      base,
		ConfigDir: filepath.Join(base, "config"),
		DataDir:   filepath.Join(base, "data"),
		StateDir:  filepath.Join(base, "run"),
		RunDir:    filepath.Join(base, "run"),
	}
}

func newQUICServer(cfg config.Config) *quic.Server {
	qs, err := quic.NewServer(cfg)
	if err != nil {
		return nil
	}
	return qs
}

func newDialer(cfg config.Config) *quic.Dialer {
	dialer, err := quic.NewDialer(cfg)
	if err != nil {
		return nil
	}
	return dialer
}

func resolveDirs() appdirs.Dirs {
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return fallbackDirs()
	}
	return dirs
}

func loadIdentityAndBook(dirs appdirs.Dirs) (identity.Identity, addressbook.Book) {
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
	return id, book
}

func loadLocalKeys(dirs appdirs.Dirs) (quic.KeyMaterial, onionkey.Keypair) {
	keys, err := quic.LoadOrCreateKeyMaterial(dirs.KeysDir())
	if err != nil {
		keys = quic.KeyMaterial{}
	}
	onion, err := onionkey.LoadOrCreate(dirs.KeysDir())
	if err != nil {
		onion = onionkey.Keypair{}
	}
	return keys, onion
}

func initObservability(cfg config.Config, dirs appdirs.Dirs) (*observability.Logger, *observability.PcapWriter, string) {
	pcapPath := dirs.PcapPath()
	pcap := observability.NewPcapWriter(cfg.Observability.PcapEnabled, pcapPath)
	logFile := cfg.Observability.LogFile
	if logFile != "" && !filepath.IsAbs(logFile) {
		logFile = filepath.Join(dirs.RunDir, logFile)
	}
	log := observability.NewWithOptions(cfg.Observability.LogFormat, logFile)
	return log, pcap, pcapPath
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

func (r *Runtime) publishRouterInfo() {
	if r == nil || r.netdb == nil || r.quic == nil {
		return
	}
	if len(r.transportKeys.PublicKey) == 0 || len(r.onionKey.Public) == 0 {
		return
	}
	addr := r.quic.Addr()
	if addr == "" {
		return
	}
	quicAddr, err := quic.ParseQUICAddr(addr)
	if err != nil {
		return
	}
	nowMs := timeutil.NowUnixMs()
	expires := nowMs + defaultRouterInfoTTL(r.reseedParams.NetDB.RecordMaxTTLMs)
	info := netdb.RouterInfo{
		V:             1,
		PeerID:        r.peerID,
		TransportPub:  r.transportKeys.PublicKey,
		OnionPub:      r.onionKey.Public,
		Addrs:         []string{quicAddr},
		Caps:          netdb.RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: nowMs,
		ExpiresAtMs:   expires,
	}
	signed, err := netdb.SignRouterInfo(r.transportKeys.PrivateKey, info)
	if err != nil {
		return
	}
	b, err := netdb.EncodeRouterInfo(signed)
	if err != nil {
		return
	}
	if status, code := r.netdb.Store(b, nowMs); status == "OK" {
		r.log.Event("info", "netdb", "netdb.routerinfo.published", r.peerID, "", "")
	} else {
		r.log.Event("warn", "netdb", "netdb.routerinfo.rejected", r.peerID, "", code)
	}
}

func (r *Runtime) startRouterInfoTicker(ctx context.Context) {
	if r == nil {
		return
	}
	interval := defaultRouterInfoTTL(r.reseedParams.NetDB.RecordMaxTTLMs) / 2
	if interval <= 0 {
		interval = int64((10 * time.Minute) / time.Millisecond)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		t := time.NewTicker(time.Duration(interval) * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.publishRouterInfo()
			}
		}
	}()
}

func defaultRouterInfoTTL(maxTTLms int64) int64 {
	if maxTTLms <= 0 {
		return int64(3600_000)
	}
	return maxTTLms
}

func (r *Runtime) applyReseed(ctx context.Context) {
	if r == nil {
		return
	}
	if !r.cfg.Reseed.Enabled {
		return
	}
	if len(r.cfg.BootstrapPeers) > 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.log.Event("info", "reseed", "reseed.fetch.start", "", "", "")
	bundle, err := reseed.FetchAndVerify(ctx, r.cfg.Reseed)
	if err != nil {
		r.net.AddDegradedReason("no_bootstrap")
		r.log.Event("warn", "reseed", "reseed.fetch.failed", "", "", err.Error())
		return
	}
	r.reseedParams = bundle.Params
	r.bootstrapPeers = bundle.SeedPeers()
	if r.netdb != nil {
		r.netdb.UpdateParams(bundle.Params.NetDB.RecordMaxTTLMs, int(bundle.Params.NetDB.K))
		nowMs := time.Now().UTC().UnixNano() / int64(time.Millisecond)
		for _, seed := range bundle.Routers {
			rec := netdb.RouterInfo{
				V:            seed.V,
				PeerID:       seed.PeerID,
				TransportPub: seed.TransportPub,
				OnionPub:     seed.OnionPub,
				Addrs:        append([]string(nil), seed.Addrs...),
				Caps: netdb.RouterCaps{
					Relay: seed.Caps.Relay,
					NetDB: seed.Caps.NetDB,
				},
				PublishedAtMs: seed.PublishedAtMs,
				ExpiresAtMs:   seed.ExpiresAtMs,
				Sig:           append([]byte(nil), seed.Sig...),
			}
			if b, err := netdb.EncodeRouterInfo(rec); err == nil {
				_, _ = r.netdb.Store(b, nowMs)
			}
		}
	}
	r.log.Event("info", "reseed", "reseed.apply.ok", "", "", "")
	if len(r.bootstrapPeers) == 0 {
		r.net.AddDegradedReason("no_bootstrap")
		r.log.Event("warn", "net", "net.degraded", "", "", "no_bootstrap")
	}
}

func (r *Runtime) dialBootstrap(ctx context.Context) {
	if r.dial == nil {
		return
	}
	peers := r.bootstrapPeers
	if len(peers) == 0 {
		peers = r.cfg.BootstrapPeers
	}
	if len(peers) == 0 {
		return
	}
	ok := 0
	for _, bp := range peers {
		for _, addr := range bp.Addrs {
			if err := r.dial.DialWithRetry(ctx, addr, bp.PeerID); err != nil {
				r.log.Event("warn", "net", "net.bootstrap_failed", bp.PeerID, "", "ERR_BOOTSTRAP_DIAL")
				continue
			}
			ok++
		}
	}
	if ok == 0 {
		r.net.AddDegradedReason("no_bootstrap")
		r.log.Event("warn", "net", "net.degraded", "", "", "no_bootstrap")
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
