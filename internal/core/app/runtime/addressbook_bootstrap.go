package runtime

import (
	"github.com/dianabuilds/ardents/internal/core/app/discovery"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
)

func (r *Runtime) addressBookBootstrapPeers(nowMs int64) []config.BootstrapPeer {
	if r == nil {
		return nil
	}
	return discovery.BootstrapPeersFromAddressBook(r.book, nowMs)
}
