package runtime

import (
	"context"
	"crypto/ed25519"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/conv"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/onionkey"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type Runtime struct {
	cfg                 config.Config
	net                 *netmgr.Manager
	dedup               *netpkg.Dedup
	bans                *netpkg.BanList
	quic                *quic.Server
	dial                *quic.Dialer
	peerID              string
	store               *storage.NodeStore
	identity            identity.Identity
	book                addressbook.Book
	log                 *observability.Logger
	pcap                *observability.PcapWriter
	pcapPath            string
	tracker             *delivery.Tracker
	health              *health.Server
	peersConnected      uint64
	metrics             *metrics.Registry
	metricsServer       *metrics.Server
	providers           *providers.Registry
	services            *serviceregistry.Registry
	tasks               *TaskStore
	sessionPeers        *sessionPeerStore
	netdb               *netdb.DB
	clockSkew           *clockSkewTracker
	clockLastNowMs      int64
	clockInvalid        uint32
	handshakeHintsSetMs int64
	powAbuse            *powAbuseTracker
	handshakeAbuse      *handshakeAbuseTracker
	dirQueryLimiter     *rateLimiter
	localCapabilities   []string
	capsMu              sync.Mutex
	peerCaps            map[string][]byte
	tunnelMu            sync.Mutex
	tunnels             map[string]*tunnelSession
	tunnelMgrMu         sync.Mutex
	outboundTunnels     []*tunnelPath
	inboundTunnels      []*tunnelPath
	tunnelMgrCancel     context.CancelFunc
	localSvcMu          sync.Mutex
	localServices       map[string]localServiceInfo
	transportKeys       quic.KeyMaterial
	onionKey            onionkey.Keypair
	relayForward        func(peerID string, envBytes []byte) error
	bootstrapPeers      []config.BootstrapPeer
	reseedParams        reseed.Params
	ipc                 *ipcServer
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
		dedup:             netpkg.NewDedup(10*time.Minute, conv.ClampUint64ToInt(cfg.Limits.MaxInflightMsgs)),
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
		sessionPeers:      newSessionPeerStore(24 * time.Hour),
		netdb:             netdb.New(netdb.DefaultRecordMaxTTLMs, netdb.DefaultK),
		clockSkew:         newClockSkewTracker(4),
		powAbuse:          newPowAbuseTracker(5),
		handshakeAbuse:    newHandshakeAbuseTracker(5),
		dirQueryLimiter:   newRateLimiter(cfg.Limits.DirQueryRateLimit, cfg.Limits.DirQueryRateWindowMs),
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
		book.RebuildIndex()
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
	if r == nil || r.net == nil {
		return
	}

	const (
		clockMinUnixMs          = int64(1577836800000) // 2020-01-01T00:00:00Z
		clockMaxUnixMs          = int64(4102444800000) // 2100-01-01T00:00:00Z
		clockMaxBackwardsJumpMs = int64(10_000)
	)

	invalid := false
	if nowMs < clockMinUnixMs || nowMs > clockMaxUnixMs {
		invalid = true
	}
	last := atomic.LoadInt64(&r.clockLastNowMs)
	if last != 0 && nowMs+clockMaxBackwardsJumpMs < last {
		invalid = true
	}
	atomic.StoreInt64(&r.clockLastNowMs, nowMs)

	prevInvalid := atomic.LoadUint32(&r.clockInvalid) == 1
	if invalid {
		// Transition into invalid state (avoid log/metric spam).
		if !prevInvalid {
			atomic.StoreUint32(&r.clockInvalid, 1)
			r.net.AddDegradedReason("clock_invalid")
			if r.metrics != nil {
				r.metrics.IncClockInvalid()
			}
			if r.log != nil {
				r.log.Event("warn", "net", "net.clock_invalid", "", "", "clock_invalid")
				r.log.Event("warn", "net", "net.degraded", "", "", "clock_invalid")
			}
		} else {
			r.net.AddDegradedReason("clock_invalid")
		}
		return
	}

	if prevInvalid {
		atomic.StoreUint32(&r.clockInvalid, 0)
		r.net.ClearDegradedReason("clock_invalid")
		if r.log != nil {
			r.log.Event("info", "net", "net.clock_ok", "", "", "")
		}
	}
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
	addrs := r.routerInfoAddrs()
	if len(addrs) == 0 {
		return
	}
	nowMs := timeutil.NowUnixMs()
	expires := nowMs + defaultRouterInfoTTL(r.reseedParams.NetDB.RecordMaxTTLMs)
	info := netdb.RouterInfo{
		V:             1,
		PeerID:        r.peerID,
		TransportPub:  r.transportKeys.PublicKey,
		OnionPub:      r.onionKey.Public,
		Addrs:         addrs,
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
	// Also advertise router.info hints as part of the handshake (SPEC-460).
	// We include "self" plus a small bounded subset of other routers from our netdb snapshot.
	hints := r.buildHandshakeRouterInfoHints(b, nowMs)
	r.quic.SetRouterInfoHints(hints)
	if r.dial != nil {
		r.dial.SetRouterInfoHints(hints)
	}
	atomic.StoreInt64(&r.handshakeHintsSetMs, nowMs)
	if status, code := r.netdb.Store(b, nowMs); status == "OK" {
		r.log.Event("info", "netdb", "netdb.routerinfo.published", r.peerID, "", "")
	} else {
		r.log.Event("warn", "netdb", "netdb.routerinfo.rejected", r.peerID, "", code)
	}
}

func (r *Runtime) refreshHandshakeRouterInfoHints(nowMs int64) {
	if r == nil || r.netdb == nil || r.quic == nil {
		return
	}

	self, ok := r.netdb.Router(r.peerID, nowMs)
	var selfBytes []byte
	if ok {
		if b, err := netdb.EncodeRouterInfo(self); err == nil {
			selfBytes = b
		}
	}
	hints := r.buildHandshakeRouterInfoHints(selfBytes, nowMs)
	r.quic.SetRouterInfoHints(hints)
	if r.dial != nil {
		r.dial.SetRouterInfoHints(hints)
	}
	r.log.Event("info", "netdb", "netdb.routerinfo.hints.updated", r.peerID, "", "count="+strconv.Itoa(len(hints)))
	atomic.StoreInt64(&r.handshakeHintsSetMs, nowMs)
}

func (r *Runtime) buildHandshakeRouterInfoHints(selfRouterInfoBytes []byte, nowMs int64) [][]byte {
	const maxHints = 8
	out := make([][]byte, 0, maxHints)
	if len(selfRouterInfoBytes) > 0 {
		out = append(out, append([]byte(nil), selfRouterInfoBytes...))
	}
	if r == nil || r.netdb == nil {
		return out
	}
	routers := r.netdb.RoutersSnapshot(nowMs)
	sort.Slice(routers, func(i, j int) bool {
		return routers[i].PeerID < routers[j].PeerID
	})
	for _, ri := range routers {
		if len(out) >= maxHints {
			break
		}
		if ri.PeerID == "" || ri.PeerID == r.peerID {
			continue
		}
		if len(ri.Addrs) == 0 {
			continue
		}
		b, err := netdb.EncodeRouterInfo(ri)
		if err != nil {
			continue
		}
		out = append(out, b)
	}
	return out
}

func (r *Runtime) routerInfoAddrs() []string {
	// Operator-configured advertised addresses take precedence.
	if r != nil && len(r.cfg.Advertise.QUICAddrs) > 0 {
		out := make([]string, 0, len(r.cfg.Advertise.QUICAddrs))
		seen := make(map[string]bool, len(r.cfg.Advertise.QUICAddrs))
		for _, a := range r.cfg.Advertise.QUICAddrs {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			hostport := a
			hostport = strings.TrimPrefix(hostport, "quic://")
			host, _, err := net.SplitHostPort(hostport)
			if err != nil {
				continue
			}
			// Wildcards are not dialable; the operator must publish a concrete host.
			if host == "0.0.0.0" || host == "::" {
				continue
			}
			norm := "quic://" + hostport
			if seen[norm] {
				continue
			}
			seen[norm] = true
			out = append(out, norm)
			if len(out) >= netdb.MaxAddrsPerRouter {
				break
			}
		}
		return out
	}

	if r == nil || r.quic == nil {
		return nil
	}
	addr := r.quic.Addr()
	if addr == "" {
		return nil
	}
	quicAddr, err := quic.ParseQUICAddr(addr)
	if err != nil {
		return nil
	}
	hostport := strings.TrimPrefix(quicAddr, "quic://")
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil
	}
	if host == "0.0.0.0" || host == "::" {
		return nil
	}
	return []string{quicAddr}
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

func (r *Runtime) startClockSkewTicker(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.checkClockSkew(timeutil.NowUnixMs())
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
		r.netdb.UpdateParams(bundle.Params.NetDB.RecordMaxTTLMs, conv.ClampUint64ToInt(bundle.Params.NetDB.K))
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
	peers := make([]config.BootstrapPeer, 0)
	if len(r.bootstrapPeers) > 0 {
		peers = append(peers, r.bootstrapPeers...)
	} else {
		peers = append(peers, r.cfg.BootstrapPeers...)
	}
	if !r.cfg.Reseed.Enabled {
		peers = mergeBootstrapPeers(peers, r.addressBookBootstrapPeers(timeutil.NowUnixMs()))
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

func (r *Runtime) observeHandshakeHint(peerID string, routerInfoBytes []byte) {
	if r == nil || r.netdb == nil || peerID == "" || len(routerInfoBytes) == 0 {
		return
	}
	// Basic anti-abuse bound: router.info records are expected to be small.
	if len(routerInfoBytes) > 16*1024 {
		return
	}
	var ri netdb.RouterInfo
	if err := codec.Unmarshal(routerInfoBytes, &ri); err != nil {
		return
	}
	if ri.PeerID == "" {
		return
	}
	nowMs := timeutil.NowUnixMs()
	status, code := r.netdb.Store(routerInfoBytes, nowMs)
	if status != "OK" {
		// Don't spam logs: handshake hints are opportunistic and may fail validation.
		_ = code
		return
	}
	// Useful for ops: allows us to confirm that netdb is being populated via handshake hints.
	// Kept at "info" but bounded by per-hello hint limits + debounce in refresh.
	r.log.Event("info", "netdb", "netdb.routerinfo.hint.stored", ri.PeerID, "", "from="+peerID)
	r.refreshHandshakeRouterInfoHints(nowMs)
}

func (r *Runtime) observePeerConnected(peerID string) {
	atomic.AddUint64(&r.peersConnected, 1)
	if r.metrics != nil {
		r.metrics.IncNetInbound()
	}
	if r.handshakeAbuse != nil {
		r.handshakeAbuse.Reset(peerID)
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
