package main

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/reseed"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func makeAuthorities(n int) ([]string, []ed25519.PrivateKey, error) {
	out := make([]string, 0, n)
	keys := make([]ed25519.PrivateKey, 0, n)
	for i := 0; i < n; i++ {
		_, priv, err := ed25519.GenerateKey(crand.Reader)
		if err != nil {
			return nil, nil, err
		}
		id, err := ids.NewIdentityID(priv.Public().(ed25519.PublicKey))
		if err != nil {
			return nil, nil, err
		}
		out = append(out, id)
		keys = append(keys, priv)
	}
	return out, keys, nil
}

func makeBundle(keys []ed25519.PrivateKey) (reseed.Bundle, error) {
	now := timeutil.NowUnixMs()
	router, err := makeRouterInfo()
	if err != nil {
		return reseed.Bundle{}, err
	}
	b := reseed.Bundle{
		V:           1,
		NetworkID:   "ardents.mainnet",
		IssuedAtMs:  now,
		ExpiresAtMs: now + int64(2*time.Hour/time.Millisecond),
		Params: reseed.Params{
			ProtocolMajor: 2,
			ProtocolMinor: 0,
			NetDB: reseed.NetDBParams{
				K:              20,
				Alpha:          3,
				Replication:    20,
				RecordMaxTTLMs: 3600_000,
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
				PowDefaultDifficulty: 16,
				RateLimits:           map[string]uint64{"netdb.store": 10},
			},
		},
		Routers: []reseed.RouterInfo{router},
	}
	signatures, err := signBundle(b, keys[:3])
	if err != nil {
		return reseed.Bundle{}, err
	}
	b.Signatures = signatures
	return b, nil
}

func makeRouterInfo() (reseed.RouterInfo, error) {
	pub, priv, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		return reseed.RouterInfo{}, err
	}
	peerID, err := ids.NewPeerID(pub)
	if err != nil {
		return reseed.RouterInfo{}, err
	}
	onionPub := make([]byte, 32)
	if _, err := crand.Read(onionPub); err != nil {
		return reseed.RouterInfo{}, err
	}
	now := timeutil.NowUnixMs()
	ri := reseed.RouterInfo{
		V:             1,
		PeerID:        peerID,
		TransportPub:  pub,
		OnionPub:      onionPub,
		Addrs:         []string{"quic://127.0.0.1:9001"},
		Caps:          reseed.RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: now,
		ExpiresAtMs:   now + 600_000,
	}
	unsigned, err := reseedUnsignedRouterBytes(ri)
	if err != nil {
		return reseed.RouterInfo{}, err
	}
	ri.Sig = ed25519.Sign(priv, unsigned)
	return ri, nil
}

func signBundle(b reseed.Bundle, keys []ed25519.PrivateKey) ([]reseed.Signature, error) {
	payload, err := reseedUnsignedBundleBytes(b)
	if err != nil {
		return nil, err
	}
	out := make([]reseed.Signature, 0, len(keys))
	for _, k := range keys {
		sig := ed25519.Sign(k, payload)
		id, err := ids.NewIdentityID(k.Public().(ed25519.PublicKey))
		if err != nil {
			return nil, err
		}
		out = append(out, reseed.Signature{
			V:                   1,
			AuthorityIdentityID: id,
			Sig:                 sig,
		})
	}
	return out, nil
}

func reseedUnsignedBundleBytes(b reseed.Bundle) ([]byte, error) {
	type unsigned struct {
		V           uint64              `cbor:"v"`
		NetworkID   string              `cbor:"network_id"`
		IssuedAtMs  int64               `cbor:"issued_at_ms"`
		ExpiresAtMs int64               `cbor:"expires_at_ms"`
		Params      reseed.Params       `cbor:"params"`
		Routers     []reseed.RouterInfo `cbor:"routers"`
	}
	u := unsigned{
		V:           b.V,
		NetworkID:   b.NetworkID,
		IssuedAtMs:  b.IssuedAtMs,
		ExpiresAtMs: b.ExpiresAtMs,
		Params:      b.Params,
		Routers:     b.Routers,
	}
	return codec.Marshal(u)
}

func reseedUnsignedRouterBytes(r reseed.RouterInfo) ([]byte, error) {
	type unsigned struct {
		V             uint64            `cbor:"v"`
		PeerID        string            `cbor:"peer_id"`
		TransportPub  []byte            `cbor:"transport_pub"`
		OnionPub      []byte            `cbor:"onion_pub"`
		Addrs         []string          `cbor:"addrs"`
		Caps          reseed.RouterCaps `cbor:"caps"`
		PublishedAtMs int64             `cbor:"published_at_ms"`
		ExpiresAtMs   int64             `cbor:"expires_at_ms"`
	}
	u := unsigned{
		V:             r.V,
		PeerID:        r.PeerID,
		TransportPub:  r.TransportPub,
		OnionPub:      r.OnionPub,
		Addrs:         r.Addrs,
		Caps:          r.Caps,
		PublishedAtMs: r.PublishedAtMs,
		ExpiresAtMs:   r.ExpiresAtMs,
	}
	return codec.Marshal(u)
}
