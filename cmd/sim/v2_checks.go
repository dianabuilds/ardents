package main

import (
	"context"
	"crypto/ed25519"
	crand "crypto/rand"
	"errors"
	"fmt"
	mrand "math/rand"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/runtime"
	netdbsvc "github.com/dianabuilds/ardents/internal/core/app/services/netdb"
	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/infra/reseed"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"golang.org/x/crypto/curve25519"
)

func checkReseedQuorum() error {
	authorities, keys, err := makeAuthorities(5)
	if err != nil {
		return err
	}
	cfg := config.Reseed{
		Enabled:     true,
		NetworkID:   "ardents.mainnet",
		URLs:        []string{"https://example.invalid/reseed"},
		Authorities: authorities,
	}
	bundle, err := makeBundle(keys)
	if err != nil {
		return err
	}
	if err := reseed.ValidateBundle(bundle, cfg); err != nil {
		return err
	}
	bundle.Signatures = bundle.Signatures[:2]
	if err := reseed.ValidateBundle(bundle, cfg); err != reseed.ErrReseedQuorumNotReached {
		return errors.New("ERR_SIM_EXPECTED_QUORUM_REJECT")
	}
	return nil
}

func checkNetDBPoisoning() error {
	db := netdb.New(netdb.DefaultRecordMaxTTLMs, netdb.DefaultK)
	nowMs := timeutil.NowUnixMs()
	pub, priv, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		return err
	}
	peerID, err := ids.NewPeerID(pub)
	if err != nil {
		return err
	}
	onionPub := make([]byte, 32)
	if _, err := crand.Read(onionPub); err != nil {
		return err
	}
	rec := netdb.RouterInfo{
		V:             1,
		PeerID:        peerID,
		TransportPub:  pub,
		OnionPub:      onionPub,
		Addrs:         []string{"quic://127.0.0.1:9001"},
		Caps:          netdb.RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: nowMs,
		ExpiresAtMs:   nowMs + 60_000,
	}
	rec, err = netdb.SignRouterInfo(priv, rec)
	if err != nil {
		return err
	}
	okBytes, err := netdb.EncodeRouterInfo(rec)
	if err != nil {
		return err
	}
	if status, _ := db.Store(okBytes, nowMs); status != "OK" {
		return errors.New("ERR_SIM_EXPECTED_OK_RECORD")
	}
	rec.Sig = []byte("bad")
	badBytes, err := netdb.EncodeRouterInfo(rec)
	if err != nil {
		return err
	}
	if status, code := db.Store(badBytes, nowMs); status != "REJECTED" || code != netdb.ErrSigInvalid.Error() {
		return errors.New("ERR_SIM_EXPECTED_SIG_REJECT")
	}
	rec, err = netdb.SignRouterInfo(priv, rec)
	if err != nil {
		return err
	}
	rec.PublishedAtMs = nowMs - 120_000
	rec.ExpiresAtMs = nowMs - 60_000
	rec, err = netdb.SignRouterInfo(priv, rec)
	if err != nil {
		return err
	}
	expBytes, err := netdb.EncodeRouterInfo(rec)
	if err != nil {
		return err
	}
	if status, code := db.Store(expBytes, nowMs); status != "REJECTED" || code != netdb.ErrExpired.Error() {
		return errors.New("ERR_SIM_EXPECTED_EXPIRED_REJECT")
	}
	return nil
}

func checkNetDBWire(rng *mrand.Rand) error {
	net := newSimNetwork(3)
	if err := net.init(); err != nil {
		return err
	}
	sender := net.peers[0]
	receiver := net.peers[1]
	nowMs := timeutil.NowUnixMs()

	rec := netdb.RouterInfo{
		V:             1,
		PeerID:        sender.PeerID(),
		TransportPub:  sender.IdentityPrivateKey().Public().(ed25519.PublicKey),
		OnionPub:      sender.SimV2OnionPublic(),
		Addrs:         []string{"quic://127.0.0.1:9001"},
		Caps:          netdb.RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: nowMs,
		ExpiresAtMs:   nowMs + 60_000,
	}
	signed, err := netdb.SignRouterInfo(sender.IdentityPrivateKey(), rec)
	if err != nil {
		return err
	}
	recBytes, err := netdb.EncodeRouterInfo(signed)
	if err != nil {
		return err
	}
	storePayload, err := netdbsvc.EncodeStore(netdbsvc.Store{V: netdbsvc.Version, Value: recBytes})
	if err != nil {
		return err
	}
	envBytes, err := buildEnvelopeV1(sender, receiver.PeerID(), netdbsvc.StoreType, storePayload)
	if err != nil {
		return err
	}
	resps, err := receiver.HandleEnvelope(sender.PeerID(), envBytes)
	if err != nil || len(resps) == 0 {
		return errors.New("ERR_SIM_NETDB_STORE_NO_REPLY")
	}
	reply, ok := getNetDBReply(resps)
	if !ok {
		if status, code := getAckStatus(resps); status != "" {
			return fmt.Errorf("netdb.store ack %s %s", status, code)
		}
		return errors.New("ERR_SIM_NETDB_STORE_MISSING_REPLY")
	}
	if reply.Status != "OK" {
		return fmt.Errorf("netdb.store %s %s", reply.Status, reply.ErrorCode)
	}
	key := dhtKey(netdb.TypeRouterInfo, rec.PeerID)
	findPayload, err := netdbsvc.EncodeFindValue(netdbsvc.FindValue{V: netdbsvc.Version, Key: key[:]})
	if err != nil {
		return err
	}
	envBytes, err = buildEnvelopeV1(sender, receiver.PeerID(), netdbsvc.FindValueType, findPayload)
	if err != nil {
		return err
	}
	resps, err = receiver.HandleEnvelope(sender.PeerID(), envBytes)
	if err != nil || len(resps) == 0 {
		return errors.New("ERR_SIM_NETDB_FIND_NO_REPLY")
	}
	reply, ok = getNetDBReply(resps)
	if !ok || reply.Status != "OK" || len(reply.Value) == 0 {
		if status, code := getAckStatus(resps); status != "" {
			return fmt.Errorf("netdb.find_value ack %s %s", status, code)
		}
		return errors.New("ERR_SIM_NETDB_FIND_EMPTY")
	}
	_ = rng
	return nil
}

func checkTunnelsAndPadding(nPeers int, rng *mrand.Rand) (int64, error) {
	if nPeers < 3 {
		return 0, errors.New("ERR_CLI_INVALID_ARGS")
	}
	net := newSimNetwork(nPeers)
	if err := net.init(); err != nil {
		return 0, err
	}

	ctx := context.Background()
	primary := net.peers[rng.Intn(len(net.peers))]
	if err := primary.SimV2RotateTunnels(ctx); err != nil {
		return 0, err
	}
	first := primary.SimV2OutboundSnapshot()
	if first == nil || len(first.HopPeerIDs) < 2 {
		return 0, errors.New("ERR_SIM_TUNNEL_OUTBOUND_MISSING")
	}
	if err := primary.SimV2RotateTunnels(ctx); err != nil {
		return 0, err
	}
	second := primary.SimV2OutboundSnapshot()
	if second == nil || len(second.HopPeerIDs) < 2 {
		return 0, errors.New("ERR_SIM_TUNNEL_OUTBOUND_MISSING_AFTER_ROTATE")
	}
	if equalTunnelSnapshots(first, second) {
		return 0, errors.New("ERR_SIM_TUNNEL_ROTATION_NO_CHANGE")
	}
	paddingSamples := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		start := time.Now()
		data, err := primary.SimV2BuildPaddingData()
		if err != nil {
			return 0, err
		}
		kind, err := runtime.SimV2PeelPadding(data, primary.SimV2OutboundSnapshot())
		if err != nil {
			return 0, err
		}
		if kind != "padding" {
			return 0, errors.New("ERR_SIM_EXPECTED_PADDING_INNER")
		}
		paddingSamples = append(paddingSamples, time.Since(start).Milliseconds())
	}
	return p95(paddingSamples), nil
}

type simNetwork struct {
	peers  []*runtime.Runtime
	byPeer map[string]*runtime.Runtime
	db     *netdb.DB
}

func newSimNetwork(n int) *simNetwork {
	return &simNetwork{
		peers:  make([]*runtime.Runtime, 0, n),
		byPeer: map[string]*runtime.Runtime{},
		db:     netdb.New(netdb.DefaultRecordMaxTTLMs, netdb.DefaultK),
	}
}

func (n *simNetwork) init() error {
	cfg := config.Default()
	nowMs := timeutil.NowUnixMs()
	count := cap(n.peers)
	for i := 0; i < count; i++ {
		id, err := identity.NewEphemeral()
		if err != nil {
			return err
		}
		peerID, err := ids.NewPeerID(id.PublicKey)
		if err != nil {
			return err
		}
		onionPriv := make([]byte, 32)
		if _, err := crand.Read(onionPriv); err != nil {
			return err
		}
		onionPub, err := curve25519.X25519(onionPriv, curve25519.Basepoint)
		if err != nil {
			return err
		}
		book := addressbook.Book{
			V:           1,
			Entries:     []addressbook.Entry{},
			UpdatedAtMs: nowMs,
		}
		book.Entries = append(book.Entries, addressbook.Entry{
			Alias:       "self",
			TargetType:  "identity",
			TargetID:    id.ID,
			Source:      "self",
			Trust:       "trusted",
			CreatedAtMs: nowMs,
		})
		rt := runtime.NewSimV2(cfg, peerID, id, book, onionkeyFrom(onionPriv, onionPub), n.db, reseed.Params{})
		n.peers = append(n.peers, rt)
		n.byPeer[peerID] = rt
	}
	for _, rt := range n.peers {
		rt := rt
		rt.SetRelayForwarder(func(peerID string, envBytes []byte) error {
			target, ok := n.byPeer[peerID]
			if !ok {
				return errors.New("ERR_SIM_PEER_MISSING")
			}
			_, _ = target.HandleEnvelope(rt.PeerID(), envBytes)
			return nil
		})
	}
	for i, rt := range n.peers {
		rec := netdb.RouterInfo{
			V:             1,
			PeerID:        rt.PeerID(),
			TransportPub:  rt.IdentityPrivateKey().Public().(ed25519.PublicKey),
			OnionPub:      rt.SimV2OnionPublic(),
			Addrs:         []string{fmt.Sprintf("quic://127.0.0.1:%d", 9000+i)},
			Caps:          netdb.RouterCaps{Relay: true, NetDB: true},
			PublishedAtMs: nowMs,
			ExpiresAtMs:   nowMs + 600_000,
		}
		signed, err := netdb.SignRouterInfo(rt.IdentityPrivateKey(), rec)
		if err != nil {
			return err
		}
		b, err := netdb.EncodeRouterInfo(signed)
		if err != nil {
			return err
		}
		if status, code := n.db.Store(b, nowMs); status != "OK" {
			return fmt.Errorf("router store failed: %s", code)
		}
	}
	return nil
}
