package reseed

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/routerinfo"
)

var (
	ErrReseedFetchFailed      = errors.New("ERR_RESEED_FETCH_FAILED")
	ErrReseedSignatureInvalid = errors.New("ERR_RESEED_SIGNATURE_INVALID")
	ErrReseedQuorumNotReached = errors.New("ERR_RESEED_QUORUM_NOT_REACHED")
	ErrReseedExpired          = errors.New("ERR_RESEED_EXPIRED")
	ErrReseedParamsInvalid    = errors.New("ERR_RESEED_PARAMS_INVALID")
	ErrReseedBundleInvalid    = errors.New("ERR_RESEED_BUNDLE_INVALID")
)

type Bundle struct {
	V           uint64       `cbor:"v"`
	NetworkID   string       `cbor:"network_id"`
	IssuedAtMs  int64        `cbor:"issued_at_ms"`
	ExpiresAtMs int64        `cbor:"expires_at_ms"`
	Params      Params       `cbor:"params"`
	Routers     []RouterInfo `cbor:"routers"`
	Signatures  []Signature  `cbor:"signatures"`
}

type Signature struct {
	V                   uint64 `cbor:"v"`
	AuthorityIdentityID string `cbor:"authority_identity_id"`
	Sig                 []byte `cbor:"sig"`
}

type Params struct {
	ProtocolMajor uint64          `cbor:"protocol_major"`
	ProtocolMinor uint64          `cbor:"protocol_minor"`
	NetDB         NetDBParams     `cbor:"netdb"`
	Tunnels       TunnelParams    `cbor:"tunnels"`
	AntiAbuse     AntiAbuseParams `cbor:"anti_abuse"`
}

type NetDBParams struct {
	K              uint64 `cbor:"k"`
	Alpha          uint64 `cbor:"alpha"`
	Replication    uint64 `cbor:"replication"`
	RecordMaxTTLMs int64  `cbor:"record_max_ttl_ms"`
}

type TunnelParams struct {
	HopCountDefault uint64 `cbor:"hop_count_default"`
	HopCountMin     uint64 `cbor:"hop_count_min"`
	HopCountMax     uint64 `cbor:"hop_count_max"`
	RotationMs      int64  `cbor:"rotation_ms"`
	LeaseTTLMs      int64  `cbor:"lease_ttl_ms"`
	PaddingPolicy   string `cbor:"padding_policy"`
}

type AntiAbuseParams struct {
	PowDefaultDifficulty uint64            `cbor:"pow_default_difficulty"`
	RateLimits           map[string]uint64 `cbor:"rate_limits"`
}

type RouterInfo struct {
	V             uint64     `cbor:"v"`
	PeerID        string     `cbor:"peer_id"`
	TransportPub  []byte     `cbor:"transport_pub"`
	OnionPub      []byte     `cbor:"onion_pub"`
	Addrs         []string   `cbor:"addrs"`
	Caps          RouterCaps `cbor:"caps"`
	PublishedAtMs int64      `cbor:"published_at_ms"`
	ExpiresAtMs   int64      `cbor:"expires_at_ms"`
	Sig           []byte     `cbor:"sig"`
}

type RouterCaps struct {
	Relay bool `cbor:"relay"`
	NetDB bool `cbor:"netdb"`
}

func FetchAndVerify(ctx context.Context, cfg config.Reseed) (Bundle, error) {
	if !cfg.Enabled {
		return Bundle{}, ErrReseedParamsInvalid
	}
	if len(cfg.URLs) == 0 || len(cfg.Authorities) == 0 {
		return Bundle{}, ErrReseedParamsInvalid
	}
	if len(cfg.Authorities) != 5 {
		return Bundle{}, ErrReseedParamsInvalid
	}
	client := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for _, url := range cfg.URLs {
		if strings.TrimSpace(url) == "" {
			continue
		}
		b, err := fetchOnce(ctx, client, url)
		if err != nil {
			lastErr = err
			continue
		}
		if err := ValidateBundle(b, cfg); err != nil {
			lastErr = err
			continue
		}
		return b, nil
	}
	if lastErr == nil {
		lastErr = ErrReseedFetchFailed
	}
	return Bundle{}, lastErr
}

func fetchOnce(ctx context.Context, client *http.Client, url string) (Bundle, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Bundle{}, ErrReseedFetchFailed
	}
	resp, err := client.Do(req)
	if err != nil {
		return Bundle{}, ErrReseedFetchFailed
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return Bundle{}, ErrReseedFetchFailed
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Bundle{}, ErrReseedFetchFailed
	}
	var b Bundle
	if err := codec.Unmarshal(data, &b); err != nil {
		return Bundle{}, ErrReseedBundleInvalid
	}
	return b, nil
}

func ValidateBundle(b Bundle, cfg config.Reseed) error {
	if b.V != 1 || b.NetworkID == "" {
		return ErrReseedBundleInvalid
	}
	if cfg.NetworkID != "" && b.NetworkID != cfg.NetworkID {
		return ErrReseedParamsInvalid
	}
	now := time.Now().UTC().UnixNano() / int64(time.Millisecond)
	if b.ExpiresAtMs <= now {
		return ErrReseedExpired
	}
	if b.ExpiresAtMs-b.IssuedAtMs > int64(24*time.Hour/time.Millisecond) {
		return ErrReseedParamsInvalid
	}
	if err := validateParams(b.Params); err != nil {
		return err
	}
	if err := verifySignatures(b, cfg.Authorities); err != nil {
		return err
	}
	if len(b.Routers) == 0 {
		return ErrReseedParamsInvalid
	}
	for _, r := range b.Routers {
		if err := validateRouterInfo(r, b.Params.NetDB.RecordMaxTTLMs); err != nil {
			return err
		}
	}
	return nil
}

func validateParams(p Params) error {
	if p.ProtocolMajor != 2 {
		return ErrReseedParamsInvalid
	}
	if p.NetDB.K != 20 || p.NetDB.Alpha != 3 || p.NetDB.Replication != 20 {
		return ErrReseedParamsInvalid
	}
	if p.NetDB.RecordMaxTTLMs != 3600_000 {
		return ErrReseedParamsInvalid
	}
	if p.Tunnels.HopCountDefault != 3 || p.Tunnels.HopCountMin != 2 || p.Tunnels.HopCountMax != 5 {
		return ErrReseedParamsInvalid
	}
	if p.Tunnels.RotationMs != 600_000 || p.Tunnels.LeaseTTLMs != 600_000 {
		return ErrReseedParamsInvalid
	}
	if p.Tunnels.PaddingPolicy != "basic.v1" {
		return ErrReseedParamsInvalid
	}
	if p.AntiAbuse.PowDefaultDifficulty == 0 {
		return ErrReseedParamsInvalid
	}
	return nil
}

func verifySignatures(b Bundle, authorities []string) error {
	allowed := map[string]bool{}
	for _, id := range authorities {
		allowed[id] = true
	}
	payload, err := unsignedBundleBytes(b)
	if err != nil {
		return ErrReseedSignatureInvalid
	}
	seen := map[string]bool{}
	valid := 0
	for _, s := range b.Signatures {
		if s.V != 1 || s.AuthorityIdentityID == "" || len(s.Sig) == 0 {
			continue
		}
		if !allowed[s.AuthorityIdentityID] || seen[s.AuthorityIdentityID] {
			continue
		}
		pub, err := ids.IdentityPublicKey(s.AuthorityIdentityID)
		if err != nil {
			continue
		}
		if ed25519.Verify(pub, payload, s.Sig) {
			seen[s.AuthorityIdentityID] = true
			valid++
		}
	}
	if valid < 3 {
		return ErrReseedQuorumNotReached
	}
	return nil
}

func unsignedBundleBytes(b Bundle) ([]byte, error) {
	type unsigned struct {
		V           uint64       `cbor:"v"`
		NetworkID   string       `cbor:"network_id"`
		IssuedAtMs  int64        `cbor:"issued_at_ms"`
		ExpiresAtMs int64        `cbor:"expires_at_ms"`
		Params      Params       `cbor:"params"`
		Routers     []RouterInfo `cbor:"routers"`
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

func validateRouterInfo(r RouterInfo, maxTTL int64) error {
	if r.V != 1 {
		return ErrReseedBundleInvalid
	}
	if err := ids.ValidatePeerID(r.PeerID); err != nil {
		return ErrReseedBundleInvalid
	}
	if len(r.TransportPub) != ed25519.PublicKeySize || len(r.OnionPub) != 32 {
		return ErrReseedBundleInvalid
	}
	if len(r.Addrs) == 0 || len(r.Addrs) > 8 {
		return ErrReseedBundleInvalid
	}
	if !r.Caps.Relay || !r.Caps.NetDB {
		return ErrReseedBundleInvalid
	}
	if r.ExpiresAtMs <= r.PublishedAtMs {
		return ErrReseedBundleInvalid
	}
	if maxTTL > 0 && (r.ExpiresAtMs-r.PublishedAtMs) > maxTTL {
		return ErrReseedBundleInvalid
	}
	unsigned, err := unsignedRouterBytes(r)
	if err != nil {
		return ErrReseedBundleInvalid
	}
	if !ed25519.Verify(r.TransportPub, unsigned, r.Sig) {
		return ErrReseedSignatureInvalid
	}
	return nil
}

func unsignedRouterBytes(r RouterInfo) ([]byte, error) {
	return routerinfo.UnsignedBytes(
		r.V,
		r.PeerID,
		r.TransportPub,
		r.OnionPub,
		r.Addrs,
		r.Caps.Relay,
		r.Caps.NetDB,
		r.PublishedAtMs,
		r.ExpiresAtMs,
	)
}

func (b Bundle) SeedPeers() []config.BootstrapPeer {
	out := make([]config.BootstrapPeer, 0, len(b.Routers))
	for _, r := range b.Routers {
		out = append(out, config.BootstrapPeer{
			PeerID: r.PeerID,
			Addrs:  append([]string(nil), r.Addrs...),
		})
	}
	return out
}

func (b Bundle) String() string {
	return fmt.Sprintf("reseed.bundle.v1 network_id=%s routers=%d", b.NetworkID, len(b.Routers))
}
