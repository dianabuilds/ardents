package discovery

import (
	"net"
	"strings"

	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/netaddr"
)

// BootstrapPeersFromAddressBook extracts trusted peer entries with inline addresses.
// Address parsing is strict (host:port); invalid entries are ignored.
func BootstrapPeersFromAddressBook(book addressbook.Book, nowMs int64) []config.BootstrapPeer {
	out := make([]config.BootstrapPeer, 0)
	seen := make(map[string]bool)
	for _, e := range book.Entries {
		if e.TargetType != "peer" || e.Trust != "trusted" {
			continue
		}
		if e.ExpiresAtMs != 0 && nowMs > e.ExpiresAtMs {
			continue
		}
		if ids.ValidatePeerID(e.TargetID) != nil {
			continue
		}
		addrs := parsePeerAddrs(e.Note)
		if len(addrs) == 0 {
			continue
		}
		if seen[e.TargetID] {
			continue
		}
		seen[e.TargetID] = true
		out = append(out, config.BootstrapPeer{PeerID: e.TargetID, Addrs: addrs})
	}
	return out
}

func parsePeerAddrs(note string) []string {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	raw := strings.FieldsFunc(note, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == ';'
	})
	out := make([]string, 0, len(raw))
	for _, addr := range raw {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		a := netaddr.StripQUICScheme(addr)
		if _, _, err := net.SplitHostPort(a); err != nil {
			continue
		}
		out = append(out, addr)
	}
	return out
}

// NormalizePeerAddrs validates addresses and returns host:port values without scheme.
func NormalizePeerAddrs(addrs []string) []string {
	if len(addrs) == 0 {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		stripped := netaddr.StripQUICScheme(addr)
		if _, _, err := net.SplitHostPort(stripped); err != nil {
			continue
		}
		out = append(out, stripped)
	}
	return out
}

// MergeBootstrapPeers merges two peer lists by peer_id, concatenating addresses.
func MergeBootstrapPeers(a, b []config.BootstrapPeer) []config.BootstrapPeer {
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]int)
	out := make([]config.BootstrapPeer, 0, len(a)+len(b))
	for _, p := range a {
		if p.PeerID == "" {
			continue
		}
		seen[p.PeerID] = len(out)
		out = append(out, p)
	}
	for _, p := range b {
		if p.PeerID == "" {
			continue
		}
		if idx, ok := seen[p.PeerID]; ok {
			out[idx].Addrs = append(out[idx].Addrs, p.Addrs...)
			continue
		}
		seen[p.PeerID] = len(out)
		out = append(out, p)
	}
	return out
}
