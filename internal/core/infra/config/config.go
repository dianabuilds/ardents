package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

type Config struct {
	NodeName       string          `json:"node_name"`
	Listen         Listen          `json:"listen"`
	BootstrapPeers []BootstrapPeer `json:"bootstrap_peers"`
	Limits         Limits          `json:"limits"`
	Pow            Pow             `json:"pow"`
	Observability  Observability   `json:"observability"`
	Reseed         Reseed          `json:"reseed"`
	Integration    Integration     `json:"integration"`
}

type Listen struct {
	QUIC     bool   `json:"quic"`
	QUICAddr string `json:"quic_addr"`
}

type BootstrapPeer struct {
	PeerID string   `json:"peer_id"`
	Addrs  []string `json:"addrs"`
}

type Limits struct {
	MaxInboundConns       uint64 `json:"max_inbound_conns"`
	MaxOutboundConns      uint64 `json:"max_outbound_conns"`
	MaxMsgBytes           uint64 `json:"max_msg_bytes"`
	MaxPayloadBytes       uint64 `json:"max_payload_bytes"`
	MaxInflightMsgs       uint64 `json:"max_inflight_msgs"`
	BanWindowMs           int64  `json:"ban_window_ms"`
	HandshakeRateLimit    uint64 `json:"handshake_rate_limit"`
	HandshakeRateWindowMs int64  `json:"handshake_rate_window_ms"`
	DirQueryRateLimit     uint64 `json:"dirquery_rate_limit"`
	DirQueryRateWindowMs  int64  `json:"dirquery_rate_window_ms"`
}

type Pow struct {
	DefaultDifficulty uint64 `json:"default_difficulty"`
}

type Observability struct {
	HealthAddr  string `json:"health_addr"`
	MetricsAddr string `json:"metrics_addr"`
	PcapEnabled bool   `json:"pcap_enabled"`
	LogFormat   string `json:"log_format"`
	LogFile     string `json:"log_file"`
}

type Reseed struct {
	Enabled     bool     `json:"enabled"`
	NetworkID   string   `json:"network_id"`
	URLs        []string `json:"urls"`
	Authorities []string `json:"authorities"`
}

type Integration struct {
	Enabled bool `json:"enabled"`
}

func Default() Config {
	return Config{
		NodeName: "peer",
		Listen: Listen{
			QUIC:     true,
			QUICAddr: "0.0.0.0:0",
		},
		BootstrapPeers: []BootstrapPeer{},
		Limits: Limits{
			MaxInboundConns:       64,
			MaxOutboundConns:      64,
			MaxMsgBytes:           256 * 1024,
			MaxPayloadBytes:       128 * 1024,
			MaxInflightMsgs:       1024,
			BanWindowMs:           int64((10 * time.Minute) / time.Millisecond),
			HandshakeRateLimit:    50,
			HandshakeRateWindowMs: int64((10 * time.Second) / time.Millisecond),
			DirQueryRateLimit:     20,
			DirQueryRateWindowMs:  int64((10 * time.Second) / time.Millisecond),
		},
		Pow: Pow{
			DefaultDifficulty: 16,
		},
		Observability: Observability{
			HealthAddr:  "127.0.0.1:8081",
			MetricsAddr: "127.0.0.1:9090",
			PcapEnabled: false,
			LogFormat:   "json",
			LogFile:     "",
		},
		Reseed: Reseed{
			Enabled:     false,
			NetworkID:   "ardents.mainnet",
			URLs:        []string{},
			Authorities: []string{},
		},
		Integration: Integration{
			Enabled: false,
		},
	}
}

func Load(path string) (Config, error) {
	if path == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			path = d.ConfigPath()
		} else {
			path = defaultConfigPath()
		}
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is controlled by app dirs.
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	ApplyDefaults(&c)
	return c, nil
}

func Save(path string, c Config) error {
	if path == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			path = d.ConfigPath()
		} else {
			path = defaultConfigPath()
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func ApplyDefaults(c *Config) {
	def := Default()
	if c.NodeName == "" {
		c.NodeName = def.NodeName
	}
	if c.Listen.QUICAddr == "" {
		c.Listen.QUICAddr = def.Listen.QUICAddr
	}
	if c.Limits.MaxMsgBytes == 0 {
		c.Limits.MaxMsgBytes = def.Limits.MaxMsgBytes
	}
	if c.Limits.MaxPayloadBytes == 0 {
		c.Limits.MaxPayloadBytes = def.Limits.MaxPayloadBytes
	}
	if c.Limits.MaxInflightMsgs == 0 {
		c.Limits.MaxInflightMsgs = def.Limits.MaxInflightMsgs
	}
	if c.Limits.MaxInboundConns == 0 {
		c.Limits.MaxInboundConns = def.Limits.MaxInboundConns
	}
	if c.Limits.MaxOutboundConns == 0 {
		c.Limits.MaxOutboundConns = def.Limits.MaxOutboundConns
	}
	if c.Limits.BanWindowMs == 0 {
		c.Limits.BanWindowMs = def.Limits.BanWindowMs
	}
	if c.Limits.HandshakeRateLimit == 0 {
		c.Limits.HandshakeRateLimit = def.Limits.HandshakeRateLimit
	}
	if c.Limits.HandshakeRateWindowMs == 0 {
		c.Limits.HandshakeRateWindowMs = def.Limits.HandshakeRateWindowMs
	}
	if c.Limits.DirQueryRateLimit == 0 {
		c.Limits.DirQueryRateLimit = def.Limits.DirQueryRateLimit
	}
	if c.Limits.DirQueryRateWindowMs == 0 {
		c.Limits.DirQueryRateWindowMs = def.Limits.DirQueryRateWindowMs
	}
	if c.Pow.DefaultDifficulty == 0 {
		c.Pow.DefaultDifficulty = def.Pow.DefaultDifficulty
	}
	if c.Observability.HealthAddr == "" {
		c.Observability.HealthAddr = def.Observability.HealthAddr
	}
	if c.Observability.MetricsAddr == "" {
		c.Observability.MetricsAddr = def.Observability.MetricsAddr
	}
	if c.Observability.LogFormat == "" {
		c.Observability.LogFormat = def.Observability.LogFormat
	}
	if c.Reseed.NetworkID == "" {
		c.Reseed.NetworkID = def.Reseed.NetworkID
	}
}

func LoadOrInit(path string) (Config, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}
	cfg = Default()
	if err := Save(path, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaultConfigPath() string {
	base := filepath.Join(os.TempDir(), "ardents")
	return filepath.Join(base, "config", "node.json")
}
