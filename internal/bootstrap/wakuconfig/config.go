package wakuconfig

import (
	"os"
	"strconv"
	"strings"
	"time"

	"aim-chat/go-backend/internal/waku"

	"gopkg.in/yaml.v3"
)

type DaemonConfig struct {
	Network DaemonNetworkConfig `yaml:"network"`
}

type DaemonNetworkConfig struct {
	Transport           string        `yaml:"transport"`
	Port                int           `yaml:"port"`
	AdvertiseAddress    string        `yaml:"advertiseAddress"`
	EnableRelay         *bool         `yaml:"enableRelay"`
	EnableStore         *bool         `yaml:"enableStore"`
	EnableFilter        *bool         `yaml:"enableFilter"`
	EnableLightPush     *bool         `yaml:"enableLightPush"`
	BootstrapNodes      []string      `yaml:"bootstrapNodes"`
	FailoverV1          *bool         `yaml:"failoverV1"`
	MinPeers            int           `yaml:"minPeers"`
	StoreQueryFanout    int           `yaml:"storeQueryFanout"`
	ReconnectInterval   time.Duration `yaml:"reconnectInterval"`
	ReconnectBackoffMax time.Duration `yaml:"reconnectBackoffMax"`
}

func LoadFromPath(configPath string) waku.Config {
	cfg := waku.DefaultConfig()

	candidates := make([]string, 0, 2)
	if configPath != "" {
		candidates = append(candidates, configPath)
	} else {
		candidates = append(candidates,
			"go-backend/configs/config.yaml",
			"configs/config.yaml",
		)
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var parsed DaemonConfig
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			continue
		}

		merged := cfg
		Merge(&merged, parsed.Network)
		ApplyEnvOverrides(&merged)
		return merged
	}

	ApplyEnvOverrides(&cfg)
	return cfg
}

func Merge(dst *waku.Config, src DaemonNetworkConfig) {
	if src.Transport != "" {
		dst.Transport = src.Transport
	}
	if src.Port != 0 {
		dst.Port = src.Port
	}
	if src.AdvertiseAddress != "" {
		dst.AdvertiseAddress = src.AdvertiseAddress
	}
	if src.EnableRelay != nil {
		dst.EnableRelay = *src.EnableRelay
	}
	if src.EnableStore != nil {
		dst.EnableStore = *src.EnableStore
	}
	if src.EnableFilter != nil {
		dst.EnableFilter = *src.EnableFilter
	}
	if src.EnableLightPush != nil {
		dst.EnableLightPush = *src.EnableLightPush
	}
	if src.BootstrapNodes != nil {
		dst.BootstrapNodes = src.BootstrapNodes
	}
	if src.FailoverV1 != nil {
		dst.FailoverV1 = *src.FailoverV1
	}
	if src.MinPeers != 0 {
		dst.MinPeers = src.MinPeers
	}
	if src.StoreQueryFanout != 0 {
		dst.StoreQueryFanout = src.StoreQueryFanout
	}
	if src.ReconnectInterval != 0 {
		dst.ReconnectInterval = src.ReconnectInterval
	}
	if src.ReconnectBackoffMax != 0 {
		dst.ReconnectBackoffMax = src.ReconnectBackoffMax
	}
}

func ApplyEnvOverrides(cfg *waku.Config) {
	if transport := strings.TrimSpace(os.Getenv("AIM_NETWORK_TRANSPORT")); transport != "" {
		cfg.Transport = transport
	}

	raw := strings.TrimSpace(os.Getenv("AIM_NETWORK_FAILOVER_V1"))
	if raw == "" {
		return
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return
	}
	cfg.FailoverV1 = v
}
