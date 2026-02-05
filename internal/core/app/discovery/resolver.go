package discovery

import (
	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
)

// Resolver provides peer address discovery across multiple sources.
type Resolver struct {
	Bootstrap   []config.BootstrapPeer
	NetDB       *netdb.DB
	SessionAddr func(peerID string) (string, bool)
}

// ResolvePeerAddr returns the first viable host:port for a peer id.
// Source order: bootstrap peers -> session cache -> NetDB router.info.
func (r Resolver) ResolvePeerAddr(peerID string, nowMs int64) (string, bool) {
	if peerID == "" {
		return "", false
	}
	if addr, ok := resolveFromBootstrap(peerID, r.Bootstrap); ok {
		return addr, true
	}
	if r.SessionAddr != nil {
		if addr, ok := r.SessionAddr(peerID); ok {
			return addr, true
		}
	}
	if r.NetDB != nil {
		if ri, ok := r.NetDB.Router(peerID, nowMs); ok {
			if addr, ok := firstValidAddr(ri.Addrs); ok {
				return addr, true
			}
		}
	}
	return "", false
}

func resolveFromBootstrap(peerID string, peers []config.BootstrapPeer) (string, bool) {
	for _, bp := range peers {
		if bp.PeerID != peerID {
			continue
		}
		if addr, ok := firstValidAddr(bp.Addrs); ok {
			return addr, true
		}
	}
	return "", false
}

func firstValidAddr(addrs []string) (string, bool) {
	norm := NormalizePeerAddrs(addrs)
	if len(norm) == 0 {
		return "", false
	}
	return norm[0], true
}
