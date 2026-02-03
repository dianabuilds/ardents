package runtime

import (
	"context"
	crand "crypto/rand"
	"sync/atomic"
	"testing"

	"crypto/ed25519"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/infra/reseed"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/onionkey"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"golang.org/x/crypto/curve25519"
)

func TestTunnelDataThreeHopDelivery(t *testing.T) {
	peers := buildSimV2Peers(t, 4)
	ctx := context.Background()
	sender := peers[0]
	wireRelayForwarders(peers)
	if err := sender.SimV2RotateTunnels(ctx); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	path := sender.outboundPathForTest()
	if path == nil {
		t.Fatal("expected outbound tunnel, got nil")
	}
	if len(path.hops) != 3 {
		t.Fatalf("expected 3-hop outbound tunnel, got %d", len(path.hops))
	}

	var forwarded int32
	wireRelayForwardersForTest(peers, &forwarded)

	inner := tunnel.Inner{V: 1, Kind: "deliver", Garlic: []byte{0x01, 0x02}}
	data, err := sender.buildTunnelData(path, inner)
	if err != nil {
		t.Fatalf("build data: %v", err)
	}
	entryPeer := path.hops[0].peerID
	envBytes := sender.buildTunnelDataEnvelope(entryPeer, data)
	if envBytes == nil {
		t.Fatalf("build tunnel envelope failed")
	}
	_, _ = sender.forwardEnvelope(entryPeer, envBytes)

	if got := atomic.LoadInt32(&forwarded); got != 3 {
		t.Fatalf("expected 3 forwards for 3-hop path, got %d", got)
	}
}

func (r *Runtime) outboundPathForTest() *tunnelPath {
	r.tunnelMgrMu.Lock()
	defer r.tunnelMgrMu.Unlock()
	if len(r.outboundTunnels) == 0 {
		return nil
	}
	return r.outboundTunnels[0]
}

func buildSimV2Peers(t *testing.T, n int) []*Runtime {
	t.Helper()
	cfg := config.Default()
	db := netdb.New(netdb.DefaultRecordMaxTTLMs, netdb.DefaultK)
	peers := make([]*Runtime, 0, n)
	nowMs := timeutil.NowUnixMs()
	idsList := make([]identity.Identity, 0, n)
	peerIDs := make([]string, 0, n)
	onions := make([]onionkey.Keypair, 0, n)
	for i := 0; i < n; i++ {
		id, err := identity.NewEphemeral()
		if err != nil {
			t.Fatalf("identity: %v", err)
		}
		peerID, err := ids.NewPeerID(id.PublicKey)
		if err != nil {
			t.Fatalf("peer id: %v", err)
		}
		idsList = append(idsList, id)
		peerIDs = append(peerIDs, peerID)
		onions = append(onions, buildOnionKeyForTest(t))
	}
	for i := 0; i < n; i++ {
		book := buildTrustBookForTest(idsList, nowMs)
		rt := NewSimV2(cfg, peerIDs[i], idsList[i], book, onions[i], db, reseed.Params{})
		peers = append(peers, rt)
	}
	seedRoutersForTest(t, db, peers, nowMs)
	return peers
}

func buildOnionKeyForTest(t *testing.T) onionkey.Keypair {
	t.Helper()
	priv := make([]byte, 32)
	if _, err := crand.Read(priv); err != nil {
		t.Fatalf("onion priv: %v", err)
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatalf("onion pub: %v", err)
	}
	return onionkey.Keypair{Private: priv, Public: pub}
}

func buildTrustBookForTest(idsList []identity.Identity, nowMs int64) addressbook.Book {
	book := addressbook.Book{V: 1, Entries: []addressbook.Entry{}, UpdatedAtMs: nowMs}
	for i, id := range idsList {
		alias := "peer"
		if i == 0 {
			alias = "self"
		}
		book.Entries = append(book.Entries, addressbook.Entry{
			Alias:       alias,
			TargetType:  "identity",
			TargetID:    id.ID,
			Source:      "self",
			Trust:       "trusted",
			CreatedAtMs: nowMs,
		})
	}
	return book
}

func seedRoutersForTest(t *testing.T, db *netdb.DB, peers []*Runtime, nowMs int64) {
	t.Helper()
	for _, rt := range peers {
		rec := netdb.RouterInfo{
			V:             1,
			PeerID:        rt.PeerID(),
			TransportPub:  rt.IdentityPrivateKey().Public().(ed25519.PublicKey),
			OnionPub:      rt.SimV2OnionPublic(),
			Addrs:         []string{"quic://127.0.0.1:9001"},
			Caps:          netdb.RouterCaps{Relay: true, NetDB: true},
			PublishedAtMs: nowMs,
			ExpiresAtMs:   nowMs + 600_000,
		}
		signed, err := netdb.SignRouterInfo(rt.IdentityPrivateKey(), rec)
		if err != nil {
			t.Fatalf("sign router: %v", err)
		}
		b, err := netdb.EncodeRouterInfo(signed)
		if err != nil {
			t.Fatalf("encode router: %v", err)
		}
		if status, code := db.Store(b, nowMs); status != "OK" {
			t.Fatalf("router store failed: %s", code)
		}
	}
}

func wireRelayForwardersForTest(peers []*Runtime, forwarded *int32) {
	byPeer := map[string]*Runtime{}
	for _, rt := range peers {
		byPeer[rt.PeerID()] = rt
	}
	for _, rt := range peers {
		rt := rt
		rt.SetRelayForwarder(func(peerID string, envBytes []byte) error {
			if forwarded != nil {
				atomic.AddInt32(forwarded, 1)
			}
			if target, ok := byPeer[peerID]; ok {
				_, _ = target.HandleEnvelope(rt.PeerID(), envBytes)
			}
			return nil
		})
	}
}

func wireRelayForwarders(peers []*Runtime) {
	wireRelayForwardersForTest(peers, nil)
}
