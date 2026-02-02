package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

const (
	DefaultConfigPath = "config/node.json"
)

type Config struct {
	NodeName       string          `json:"node_name"`
	Listen         Listen          `json:"listen"`
	BootstrapPeers []BootstrapPeer `json:"bootstrap_peers"`
	Limits         Limits          `json:"limits"`
	Pow            Pow             `json:"pow"`
	Observability  Observability   `json:"observability"`
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
	MaxInboundConns  uint64 `json:"max_inbound_conns"`
	MaxOutboundConns uint64 `json:"max_outbound_conns"`
	MaxMsgBytes      uint64 `json:"max_msg_bytes"`
	MaxPayloadBytes  uint64 `json:"max_payload_bytes"`
	MaxInflightMsgs  uint64 `json:"max_inflight_msgs"`
	BanWindowMs      int64  `json:"ban_window_ms"`
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

func Default() Config {
	return Config{
		NodeName: "peer",
		Listen: Listen{
			QUIC:     true,
			QUICAddr: "0.0.0.0:0",
		},
		BootstrapPeers: []BootstrapPeer{},
		Limits: Limits{
			MaxInboundConns:  64,
			MaxOutboundConns: 64,
			MaxMsgBytes:      256 * 1024,
			MaxPayloadBytes:  128 * 1024,
			MaxInflightMsgs:  1024,
			BanWindowMs:      int64((10 * time.Minute) / time.Millisecond),
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
	}
}

func Load(path string) (Config, error) {
	if path == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			path = d.ConfigPath()
		} else {
			path = DefaultConfigPath
		}
	}
	data, err := os.ReadFile(path)
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
			path = DefaultConfigPath
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
}
