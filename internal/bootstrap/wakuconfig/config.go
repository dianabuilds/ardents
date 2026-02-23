package wakuconfig

import (
	"aim-chat/go-backend/internal/bootstrap/bootstrapmanager"
	"log/slog"
	"os"
	"path/filepath"
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
	Transport                  string        `yaml:"transport"`
	Port                       int           `yaml:"port"`
	AdvertiseAddress           string        `yaml:"advertiseAddress"`
	EnableRelay                *bool         `yaml:"enableRelay"`
	EnableStore                *bool         `yaml:"enableStore"`
	EnableFilter               *bool         `yaml:"enableFilter"`
	EnableLightPush            *bool         `yaml:"enableLightPush"`
	BootstrapNodes             []string      `yaml:"bootstrapNodes"`
	FailoverV1                 *bool         `yaml:"failoverV1"`
	MinPeers                   int           `yaml:"minPeers"`
	StoreQueryFanout           int           `yaml:"storeQueryFanout"`
	ReconnectInterval          time.Duration `yaml:"reconnectInterval"`
	ReconnectBackoffMax        time.Duration `yaml:"reconnectBackoffMax"`
	ManifestRefreshInterval    time.Duration `yaml:"manifestRefreshInterval"`
	ManifestStaleWindow        time.Duration `yaml:"manifestStaleWindow"`
	ManifestRefreshTimeout     time.Duration `yaml:"manifestRefreshTimeout"`
	ManifestBackoffBase        time.Duration `yaml:"manifestBackoffBase"`
	ManifestBackoffMax         time.Duration `yaml:"manifestBackoffMax"`
	ManifestBackoffFactor      float64       `yaml:"manifestBackoffFactor"`
	ManifestBackoffJitterRatio float64       `yaml:"manifestBackoffJitterRatio"`
}

//func LoadFromPath(configPath string) waku.Config {
//	return LoadFromPathWithDataDir(configPath, "")
//}

func LoadFromPathWithDataDir(configPath, dataDir string) waku.Config {
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
		applyBootstrapManager(&merged, dataDir)
		return merged
	}

	ApplyEnvOverrides(&cfg)
	applyBootstrapManager(&cfg, dataDir)
	return cfg
}

func Merge(dst *waku.Config, src DaemonNetworkConfig) {
	if src.Transport != "" {
		dst.Transport = src.Transport
	}
	mergeIfSet(&dst.Port, src.Port)
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
	mergeIfSet(&dst.MinPeers, src.MinPeers)
	mergeIfSet(&dst.StoreQueryFanout, src.StoreQueryFanout)
	mergeIfSet(&dst.ReconnectInterval, src.ReconnectInterval)
	mergeIfSet(&dst.ReconnectBackoffMax, src.ReconnectBackoffMax)
	mergeIfSet(&dst.ManifestRefreshInterval, src.ManifestRefreshInterval)
	mergeIfSet(&dst.ManifestStaleWindow, src.ManifestStaleWindow)
	mergeIfSet(&dst.ManifestRefreshTimeout, src.ManifestRefreshTimeout)
	mergeIfSet(&dst.ManifestBackoffBase, src.ManifestBackoffBase)
	mergeIfSet(&dst.ManifestBackoffMax, src.ManifestBackoffMax)
	mergeIfSet(&dst.ManifestBackoffFactor, src.ManifestBackoffFactor)
	mergeIfSet(&dst.ManifestBackoffJitterRatio, src.ManifestBackoffJitterRatio)
}

func mergeIfSet[T comparable](dst *T, src T) {
	var zero T
	if src != zero {
		*dst = src
	}
}

func ApplyEnvOverrides(cfg *waku.Config) {
	if transport := strings.TrimSpace(os.Getenv("AIM_NETWORK_TRANSPORT")); transport != "" {
		cfg.Transport = transport
	}

	raw := strings.TrimSpace(os.Getenv("AIM_NETWORK_FAILOVER_V1"))
	if raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			cfg.FailoverV1 = v
		}
	}

	if refreshInterval := strings.TrimSpace(os.Getenv("AIM_MANIFEST_REFRESH_INTERVAL")); refreshInterval != "" {
		if d, err := time.ParseDuration(refreshInterval); err == nil {
			cfg.ManifestRefreshInterval = d
		}
	}
	if staleWindow := strings.TrimSpace(os.Getenv("AIM_MANIFEST_STALE_WINDOW")); staleWindow != "" {
		if d, err := time.ParseDuration(staleWindow); err == nil {
			cfg.ManifestStaleWindow = d
		}
	}
	if refreshTimeout := strings.TrimSpace(os.Getenv("AIM_MANIFEST_REFRESH_TIMEOUT")); refreshTimeout != "" {
		if d, err := time.ParseDuration(refreshTimeout); err == nil {
			cfg.ManifestRefreshTimeout = d
		}
	}
	if backoffBase := strings.TrimSpace(os.Getenv("AIM_MANIFEST_BACKOFF_BASE")); backoffBase != "" {
		if d, err := time.ParseDuration(backoffBase); err == nil {
			cfg.ManifestBackoffBase = d
		}
	}
	if backoffMax := strings.TrimSpace(os.Getenv("AIM_MANIFEST_BACKOFF_MAX")); backoffMax != "" {
		if d, err := time.ParseDuration(backoffMax); err == nil {
			cfg.ManifestBackoffMax = d
		}
	}
	if backoffFactor := strings.TrimSpace(os.Getenv("AIM_MANIFEST_BACKOFF_FACTOR")); backoffFactor != "" {
		if v, err := strconv.ParseFloat(backoffFactor, 64); err == nil {
			cfg.ManifestBackoffFactor = v
		}
	}
	if backoffJitter := strings.TrimSpace(os.Getenv("AIM_MANIFEST_BACKOFF_JITTER_RATIO")); backoffJitter != "" {
		if v, err := strconv.ParseFloat(backoffJitter, 64); err == nil {
			cfg.ManifestBackoffJitterRatio = v
		}
	}
}

func applyBootstrapManager(cfg *waku.Config, dataDir string) {
	baked := bootstrapmanager.BootstrapSet{
		Source:         bootstrapmanager.SourceBaked,
		BootstrapNodes: append([]string(nil), cfg.BootstrapNodes...),
		MinPeers:       cfg.MinPeers,
		ReconnectPolicy: bootstrapmanager.ReconnectPolicy{
			BaseIntervalMS: int(cfg.ReconnectInterval / time.Millisecond),
			MaxIntervalMS:  int(cfg.ReconnectBackoffMax / time.Millisecond),
			JitterRatio:    0.2,
		},
	}

	manifestPath := strings.TrimSpace(os.Getenv("AIM_NETWORK_MANIFEST_PATH"))
	trustBundlePath := strings.TrimSpace(os.Getenv("AIM_TRUST_BUNDLE_PATH"))
	cachePath := resolveBootstrapCachePath(dataDir)
	if override := strings.TrimSpace(os.Getenv("AIM_BOOTSTRAP_CACHE_PATH")); override != "" {
		cachePath = override
	}

	manager := bootstrapmanager.New(manifestPath, trustBundlePath, cachePath, baked)
	cfg.BootstrapManifestPath = manifestPath
	cfg.BootstrapTrustBundlePath = trustBundlePath
	cfg.BootstrapCachePath = cachePath
	load := manager.LoadBootstrapSet()
	if !load.OK || load.Set == nil {
		cfg.BootstrapSource = "unavailable"
		slog.Warn("manifest.verify.rejected",
			"event_type", "manifest.verify.rejected",
			"result", "rejected",
			"reject_code", manager.LastRejectCode(),
			"reason", manager.LastReason(),
		)
		return
	}
	apply := manager.ApplyBootstrapSet(cfg, *load.Set)
	if !apply.Applied {
		cfg.BootstrapSource = "unavailable"
		slog.Warn("bootstrap.apply.failed",
			"event_type", "bootstrap.apply",
			"result", "rejected",
			"error_code", apply.ErrorCode,
			"reason", apply.Reason,
		)
		return
	}
	if load.Set.Source == bootstrapmanager.SourceManifest && load.Set.ManifestMeta != nil {
		slog.Info("manifest.verify.accepted",
			"event_type", "manifest.verify.accepted",
			"result", "accepted",
			"manifest_version", load.Set.ManifestMeta.Version,
			"manifest_key_id", load.Set.ManifestMeta.KeyID,
		)
	}
}

func resolveBootstrapCachePath(dataDir string) string {
	baseDir := strings.TrimSpace(dataDir)
	if baseDir == "" {
		baseDir = "."
	}
	return filepath.Join(baseDir, "network", "bootstrap-cache.json")
}
