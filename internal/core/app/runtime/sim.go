package runtime

import (
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
	"github.com/dianabuilds/ardents/internal/core/transport/quic"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/onionkey"
)

func NewSim(cfg config.Config, peerID string, id identity.Identity, book addressbook.Book) *Runtime {
	rt := newSimRuntime(cfg, peerID, id, book)
	rt.transportKeys = quic.KeyMaterial{
		PrivateKey: id.PrivateKey,
		PublicKey:  id.PublicKey,
	}
	return rt
}

func NewSimV2(cfg config.Config, peerID string, id identity.Identity, book addressbook.Book, onion onionkey.Keypair, db *netdb.DB, params reseed.Params) *Runtime {
	peerID = ensurePeerID(peerID, id)
	if db == nil {
		db = netdb.New(netdb.DefaultRecordMaxTTLMs, netdb.DefaultK)
	}
	if params.ProtocolMajor == 0 {
		params = defaultReseedParams(cfg)
	}
	rt := newSimRuntime(cfg, peerID, id, book)
	rt.netdb = db
	rt.clockSkew = newClockSkewTracker(4)
	rt.powAbuse = newPowAbuseTracker(5)
	rt.localCapabilities = []string{"node.fetch.v1", "dir.query.v1"}
	rt.peerCaps = make(map[string][]byte)
	rt.tunnels = make(map[string]*tunnelSession)
	rt.localServices = make(map[string]localServiceInfo)
	rt.transportKeys = quic.KeyMaterial{
		PrivateKey: id.PrivateKey,
		PublicKey:  id.PublicKey,
	}
	rt.onionKey = onion
	rt.reseedParams = params
	return rt
}

func newSimRuntime(cfg config.Config, peerID string, id identity.Identity, book addressbook.Book) *Runtime {
	peerID = ensurePeerID(peerID, id)
	return &Runtime{
		cfg:       cfg,
		net:       netmgr.New(),
		dedup:     netpkg.NewDedup(10*time.Minute, int(cfg.Limits.MaxInflightMsgs)),
		bans:      netpkg.NewBanList(),
		peerID:    peerID,
		store:     storage.NewNodeStore(1_048_576),
		identity:  id,
		book:      book,
		log:       observability.New(),
		tracker:   delivery.NewTracker(),
		metrics:   metrics.New(),
		providers: providers.NewRegistry(),
		services:  serviceregistry.New(),
		tasks:     NewTaskStore(24 * time.Hour),
	}
}

func ensurePeerID(peerID string, id identity.Identity) string {
	if peerID != "" || id.PublicKey == nil {
		return peerID
	}
	if pid, err := ids.NewPeerID(id.PublicKey); err == nil {
		return pid
	}
	return peerID
}

func defaultReseedParams(cfg config.Config) reseed.Params {
	params := reseed.Params{
		ProtocolMajor: 2,
		ProtocolMinor: 0,
		NetDB: reseed.NetDBParams{
			K:              20,
			Alpha:          3,
			Replication:    20,
			RecordMaxTTLMs: netdb.DefaultRecordMaxTTLMs,
		},
		Tunnels: reseed.TunnelParams{
			HopCountDefault: 3,
			HopCountMin:     2,
			HopCountMax:     5,
			RotationMs:      600_000,
			LeaseTTLMs:      600_000,
			PaddingPolicy:   "basic.v1",
		},
		AntiAbuse: reseed.AntiAbuseParams{
			PowDefaultDifficulty: cfg.Pow.DefaultDifficulty,
			RateLimits:           map[string]uint64{"netdb.store": 10},
		},
	}
	if params.AntiAbuse.PowDefaultDifficulty == 0 {
		params.AntiAbuse.PowDefaultDifficulty = 16
	}
	return params
}

func (r *Runtime) HandleEnvelope(fromPeerID string, data []byte) ([][]byte, error) {
	return r.handleEnvelope(fromPeerID, data)
}

func (r *Runtime) Store() *storage.NodeStore {
	return r.store
}

func (r *Runtime) Tasks() *TaskStore {
	return r.tasks
}
