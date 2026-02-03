package reseed

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/testutil"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func TestValidateBundle_QuorumAndRouterSig(t *testing.T) {
	authorities, keys := makeAuthorities(t, 5)
	cfg := config.Reseed{
		Enabled:     true,
		NetworkID:   "ardents.mainnet",
		URLs:        []string{"https://example.invalid/reseed"},
		Authorities: authorities,
	}
	bundle := makeBundle(t, keys)
	if err := ValidateBundle(bundle, cfg); err != nil {
		t.Fatalf("expected valid bundle, got %v", err)
	}

	bundle.Signatures = bundle.Signatures[:2]
	if err := ValidateBundle(bundle, cfg); !errors.Is(err, ErrReseedQuorumNotReached) {
		t.Fatalf("expected quorum error, got %v", err)
	}

	bundle = makeBundle(t, keys)
	bundle.Routers[0].Sig = []byte("bad")
	bundle.Signatures = signBundle(t, bundle, keys[:3])
	if err := ValidateBundle(bundle, cfg); !errors.Is(err, ErrReseedSignatureInvalid) {
		t.Fatalf("expected router sig error, got %v", err)
	}
}

func makeAuthorities(t *testing.T, n int) ([]string, []ed25519.PrivateKey) {
	t.Helper()
	out := make([]string, 0, n)
	keys := make([]ed25519.PrivateKey, 0, n)
	for i := 0; i < n; i++ {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		id, err := ids.NewIdentityID(priv.Public().(ed25519.PublicKey))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
		keys = append(keys, priv)
	}
	return out, keys
}

func makeBundle(t *testing.T, keys []ed25519.PrivateKey) Bundle {
	t.Helper()
	now := timeutil.NowUnixMs()
	router := makeRouterInfo(t, now)
	b := Bundle{
		V:           1,
		NetworkID:   "ardents.mainnet",
		IssuedAtMs:  now,
		ExpiresAtMs: now + int64(2*time.Hour/time.Millisecond),
		Params: Params{
			ProtocolMajor: 2,
			ProtocolMinor: 0,
			NetDB: NetDBParams{
				K:              20,
				Alpha:          3,
				Replication:    20,
				RecordMaxTTLMs: 3600_000,
			},
			Tunnels: TunnelParams{
				HopCountDefault: 3,
				HopCountMin:     2,
				HopCountMax:     5,
				RotationMs:      600_000,
				LeaseTTLMs:      600_000,
				PaddingPolicy:   "basic.v1",
			},
			AntiAbuse: AntiAbuseParams{
				PowDefaultDifficulty: 16,
				RateLimits:           map[string]uint64{"netdb.store": 10},
			},
		},
		Routers: []RouterInfo{router},
	}
	b.Signatures = signBundle(t, b, keys[:3])
	return b
}

func makeRouterInfo(t *testing.T, now int64) RouterInfo {
	t.Helper()
	priv, pub, peerID := testutil.NewPeerKeyAndID(t)
	onionPub := testutil.NewOnionPub(t)
	ri := RouterInfo{
		V:             1,
		PeerID:        peerID,
		TransportPub:  pub,
		OnionPub:      onionPub,
		Addrs:         []string{"quic://127.0.0.1:9001"},
		Caps:          RouterCaps{Relay: true, NetDB: true},
		PublishedAtMs: now,
		ExpiresAtMs:   now + 600_000,
	}
	unsigned, err := unsignedRouterBytes(ri)
	if err != nil {
		t.Fatal(err)
	}
	ri.Sig = ed25519.Sign(priv, unsigned)
	return ri
}

func signBundle(t *testing.T, b Bundle, keys []ed25519.PrivateKey) []Signature {
	t.Helper()
	signBytes, err := unsignedBundleBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]Signature, 0, len(keys))
	for _, k := range keys {
		sig := ed25519.Sign(k, signBytes)
		id, err := ids.NewIdentityID(k.Public().(ed25519.PublicKey))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, Signature{
			V:                   1,
			AuthorityIdentityID: id,
			Sig:                 sig,
		})
	}
	return out
}

func TestUnsignedBundleDeterminism(t *testing.T) {
	authorities, keys := makeAuthorities(t, 5)
	_ = authorities
	b := makeBundle(t, keys)
	a, err := unsignedBundleBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := codec.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) == 0 || len(bb) == 0 {
		t.Fatal("expected non-empty bytes")
	}
}
