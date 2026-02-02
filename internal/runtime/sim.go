package runtime

import (
	"time"

	"github.com/dianabuilds/ardents/internal/addressbook"
	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/delivery"
	"github.com/dianabuilds/ardents/internal/metrics"
	netpkg "github.com/dianabuilds/ardents/internal/net"
	"github.com/dianabuilds/ardents/internal/netmgr"
	"github.com/dianabuilds/ardents/internal/observability"
	"github.com/dianabuilds/ardents/internal/providers"
	"github.com/dianabuilds/ardents/internal/services/serviceregistry"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/storage"
	"github.com/dianabuilds/ardents/internal/transport/quic"
)

func NewSim(cfg config.Config, peerID string, id identity.Identity, book addressbook.Book) *Runtime {
	if peerID == "" && id.PublicKey != nil {
		if pid, err := ids.NewPeerID(id.PublicKey); err == nil {
			peerID = pid
		}
	}
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
		transportKeys: quic.KeyMaterial{
			PrivateKey: id.PrivateKey,
			PublicKey:  id.PublicKey,
		},
	}
}

func (r *Runtime) HandleEnvelope(fromPeerID string, data []byte) ([][]byte, error) {
	return r.handleEnvelope(fromPeerID, data)
}

func (r *Runtime) Store() *storage.NodeStore {
	return r.store
}
