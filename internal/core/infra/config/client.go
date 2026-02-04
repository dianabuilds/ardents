package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

var (
	ErrClientConfigInvalid = errors.New("ERR_CLIENT_CONFIG_INVALID")
)

type ClientConfig struct {
	V                 uint64       `json:"v"`
	BootstrapPeers    []ClientPeer `json:"bootstrap_peers"`
	TrustedIdentities []string     `json:"trusted_identities"`
	RefreshMs         int64        `json:"refresh_ms"`
	Reseed            ClientReseed `json:"reseed"`
	Limits            ClientLimits `json:"limits"`
}

type ClientPeer struct {
	PeerID     string   `json:"peer_id"`
	Addrs      []string `json:"addrs"`
	IdentityID string   `json:"identity_id,omitempty"`
}

type ClientReseed struct {
	Enabled     bool     `json:"enabled"`
	NetworkID   string   `json:"network_id"`
	URLs        []string `json:"urls"`
	Authorities []string `json:"authorities"`
}

type ClientLimits struct {
	MaxPeers           uint64 `json:"max_peers"`
	AddRateLimit       uint64 `json:"add_rate_limit"`
	AddRateWindowMs    int64  `json:"add_rate_window_ms"`
	CooldownBaseMs     int64  `json:"cooldown_base_ms"`
	CooldownMaxMs      int64  `json:"cooldown_max_ms"`
	HandshakeHintTTLMs int64  `json:"handshake_hint_ttl_ms"`
}

func DefaultClient() ClientConfig {
	return ClientConfig{
		V:                 1,
		BootstrapPeers:    []ClientPeer{},
		TrustedIdentities: []string{},
		RefreshMs:         int64((5 * time.Minute) / time.Millisecond),
		Reseed: ClientReseed{
			Enabled:     false,
			NetworkID:   "ardents.mainnet",
			URLs:        []string{},
			Authorities: []string{},
		},
		Limits: ClientLimits{
			MaxPeers:           512,
			AddRateLimit:       50,
			AddRateWindowMs:    int64((10 * time.Second) / time.Millisecond),
			CooldownBaseMs:     int64((2 * time.Second) / time.Millisecond),
			CooldownMaxMs:      int64((60 * time.Second) / time.Millisecond),
			HandshakeHintTTLMs: int64((10 * time.Second) / time.Millisecond),
		},
	}
}

func LoadClient(path string) (ClientConfig, error) {
	if path == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			path = d.ClientConfigPath()
		} else {
			path = filepath.Join(os.TempDir(), "ardents", "config", "client.json")
		}
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is controlled by app dirs.
	if err != nil {
		return ClientConfig{}, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // tolerate UTF-8 BOM (common on Windows)
	var c ClientConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return ClientConfig{}, fmt.Errorf("%w: %v", ErrClientConfigInvalid, err)
	}
	ApplyClientDefaults(&c)
	if err := validateClientConfig(c); err != nil {
		return ClientConfig{}, err
	}
	return c, nil
}

func ApplyClientDefaults(c *ClientConfig) {
	def := DefaultClient()
	if c.V == 0 {
		c.V = def.V
	}
	if c.RefreshMs == 0 {
		c.RefreshMs = def.RefreshMs
	}
	if c.Reseed.NetworkID == "" {
		c.Reseed.NetworkID = def.Reseed.NetworkID
	}
	if c.Limits.MaxPeers == 0 {
		c.Limits.MaxPeers = def.Limits.MaxPeers
	}
	if c.Limits.AddRateLimit == 0 {
		c.Limits.AddRateLimit = def.Limits.AddRateLimit
	}
	if c.Limits.AddRateWindowMs == 0 {
		c.Limits.AddRateWindowMs = def.Limits.AddRateWindowMs
	}
	if c.Limits.CooldownBaseMs == 0 {
		c.Limits.CooldownBaseMs = def.Limits.CooldownBaseMs
	}
	if c.Limits.CooldownMaxMs == 0 {
		c.Limits.CooldownMaxMs = def.Limits.CooldownMaxMs
	}
	if c.Limits.HandshakeHintTTLMs == 0 {
		c.Limits.HandshakeHintTTLMs = def.Limits.HandshakeHintTTLMs
	}
}

func validateClientConfig(c ClientConfig) error {
	if c.V != 1 {
		return ErrClientConfigInvalid
	}
	if c.RefreshMs < 60_000 {
		return ErrClientConfigInvalid
	}
	if c.Limits.MaxPeers == 0 || c.Limits.MaxPeers > 4096 {
		return ErrClientConfigInvalid
	}
	if c.Limits.AddRateWindowMs <= 0 {
		return ErrClientConfigInvalid
	}
	if c.Limits.CooldownBaseMs <= 0 || c.Limits.CooldownMaxMs < c.Limits.CooldownBaseMs {
		return ErrClientConfigInvalid
	}
	if c.Limits.HandshakeHintTTLMs <= 0 {
		return ErrClientConfigInvalid
	}
	for _, bp := range c.BootstrapPeers {
		if bp.PeerID == "" || len(bp.Addrs) == 0 {
			return ErrClientConfigInvalid
		}
	}
	return nil
}
