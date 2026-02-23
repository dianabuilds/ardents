package daemonservice

import (
	"errors"
	"strings"

	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/pkg/models"
)

type blobNodePreset string

const (
	blobNodePresetCustom blobNodePreset = "custom"
	blobNodePresetAssist blobNodePreset = "assist"
	blobNodePresetLite   blobNodePreset = "lite"
	blobNodePresetCache  blobNodePreset = "cache"
	blobNodePresetPin    blobNodePreset = "pin"
)

const nodeProfileNetworkAssistDefault = "network_assist_default"

type blobNodePresetConfig struct {
	Preset                     blobNodePreset
	ProfileID                  string
	StorageProtection          string
	Retention                  string
	ImageQuotaMB               int
	FileQuotaMB                int
	ImageMaxItemSizeMB         int
	FileMaxItemSizeMB          int
	ReplicationMode            string
	RelayEnabled               bool
	PublicDiscoveryEnabled     bool
	PublicServingEnabled       bool
	PublicStoreEnabled         bool
	PersonalStoreEnabled       bool
	AnnounceEnabled            bool
	FetchEnabled               bool
	RolloutPercent             int
	ServeBandwidthKBps         int
	ServeBandwidthSoftKBps     int
	ServeBandwidthHardKBps     int
	ServeMaxConcurrent         int
	ServeRequestsPerMinPerPeer int
	PublicEphemeralCacheMaxMB  int
	PublicEphemeralCacheTTLMin int
	FetchBandwidthKBps         int
	HighWatermarkPercent       int
	FullCapPercent             int
	AggressiveTargetPercent    int
}

var errInvalidBlobNodePreset = errors.New("invalid blob node preset")

func defaultBlobNodePresetConfig() blobNodePresetConfig {
	return buildBlobNodePresetConfig(blobNodePresetAssist)
}

func defaultBlobNodePresetProfile() blobNodePresetConfig {
	return blobNodePresetConfig{
		Preset:                     blobNodePresetCustom,
		ProfileID:                  nodeProfileNetworkAssistDefault,
		StorageProtection:          string(privacydomain.StorageProtectionStandard),
		Retention:                  string(privacydomain.RetentionPersistent),
		ImageQuotaMB:               0,
		FileQuotaMB:                0,
		ImageMaxItemSizeMB:         0,
		FileMaxItemSizeMB:          0,
		ReplicationMode:            string(blobReplicationModeOnDemand),
		RelayEnabled:               true,
		PublicDiscoveryEnabled:     true,
		PublicServingEnabled:       true,
		PublicStoreEnabled:         false,
		PersonalStoreEnabled:       true,
		AnnounceEnabled:            true,
		FetchEnabled:               true,
		RolloutPercent:             100,
		ServeBandwidthKBps:         0,
		ServeBandwidthSoftKBps:     0,
		ServeBandwidthHardKBps:     0,
		ServeMaxConcurrent:         0,
		ServeRequestsPerMinPerPeer: 0,
		PublicEphemeralCacheMaxMB:  256,
		PublicEphemeralCacheTTLMin: 30,
		FetchBandwidthKBps:         0,
		HighWatermarkPercent:       90,
		FullCapPercent:             100,
		AggressiveTargetPercent:    75,
	}
}

func parseBlobNodePreset(raw string) (blobNodePreset, error) {
	switch blobNodePreset(strings.ToLower(strings.TrimSpace(raw))) {
	case blobNodePresetCustom:
		return blobNodePresetCustom, nil
	case blobNodePresetAssist:
		return blobNodePresetAssist, nil
	case blobNodePresetLite:
		return blobNodePresetLite, nil
	case blobNodePresetCache:
		return blobNodePresetCache, nil
	case blobNodePresetPin:
		return blobNodePresetPin, nil
	default:
		return "", errInvalidBlobNodePreset
	}
}

func buildBlobNodePresetConfig(preset blobNodePreset) blobNodePresetConfig {
	cfg := defaultBlobNodePresetProfile()
	cfg.Preset = preset
	switch preset {
	case blobNodePresetAssist:
		cfg.ImageQuotaMB = 1024
		cfg.FileQuotaMB = 9216
		cfg.ImageMaxItemSizeMB = 20
		cfg.FileMaxItemSizeMB = 100
		cfg.ReplicationMode = string(blobReplicationModeOnDemand)
		cfg.AnnounceEnabled = true
		cfg.FetchEnabled = true
		cfg.ServeBandwidthKBps = 512
		cfg.ServeBandwidthSoftKBps = 512
		cfg.ServeBandwidthHardKBps = 1024
		cfg.ServeMaxConcurrent = 3
		cfg.ServeRequestsPerMinPerPeer = 120
		cfg.PublicEphemeralCacheMaxMB = 256
		cfg.PublicEphemeralCacheTTLMin = 30
		cfg.FetchBandwidthKBps = 1024
		cfg.HighWatermarkPercent = 85
		cfg.AggressiveTargetPercent = 70
	case blobNodePresetLite:
		cfg.ImageQuotaMB = 256
		cfg.FileQuotaMB = 512
		cfg.ImageMaxItemSizeMB = 8
		cfg.FileMaxItemSizeMB = 16
		cfg.ReplicationMode = string(blobReplicationModeNone)
		cfg.AnnounceEnabled = false
		cfg.FetchEnabled = true
		cfg.ServeBandwidthKBps = 256
		cfg.ServeBandwidthSoftKBps = 256
		cfg.ServeBandwidthHardKBps = 512
		cfg.ServeMaxConcurrent = 1
		cfg.ServeRequestsPerMinPerPeer = 60
		cfg.PublicEphemeralCacheMaxMB = 128
		cfg.PublicEphemeralCacheTTLMin = 20
		cfg.FetchBandwidthKBps = 512
		cfg.HighWatermarkPercent = 85
		cfg.AggressiveTargetPercent = 65
	case blobNodePresetCache:
		cfg.ImageQuotaMB = 1024
		cfg.FileQuotaMB = 4096
		cfg.ImageMaxItemSizeMB = 16
		cfg.FileMaxItemSizeMB = 64
		cfg.ReplicationMode = string(blobReplicationModeOnDemand)
		cfg.AnnounceEnabled = true
		cfg.FetchEnabled = true
		cfg.ServeBandwidthKBps = 1024
		cfg.ServeBandwidthSoftKBps = 1024
		cfg.ServeBandwidthHardKBps = 2048
		cfg.ServeMaxConcurrent = 4
		cfg.ServeRequestsPerMinPerPeer = 180
		cfg.PublicEphemeralCacheMaxMB = 256
		cfg.PublicEphemeralCacheTTLMin = 30
		cfg.FetchBandwidthKBps = 2048
		cfg.HighWatermarkPercent = 90
		cfg.AggressiveTargetPercent = 75
	case blobNodePresetPin:
		cfg.ImageQuotaMB = 2048
		cfg.FileQuotaMB = 8192
		cfg.ImageMaxItemSizeMB = 32
		cfg.FileMaxItemSizeMB = 128
		cfg.ReplicationMode = string(blobReplicationModePinnedOnly)
		cfg.AnnounceEnabled = true
		cfg.FetchEnabled = true
		cfg.ServeBandwidthKBps = 4096
		cfg.ServeBandwidthSoftKBps = 4096
		cfg.ServeBandwidthHardKBps = 8192
		cfg.ServeMaxConcurrent = 6
		cfg.ServeRequestsPerMinPerPeer = 240
		cfg.PublicEphemeralCacheMaxMB = 512
		cfg.PublicEphemeralCacheTTLMin = 45
		cfg.FetchBandwidthKBps = 4096
		cfg.HighWatermarkPercent = 90
		cfg.AggressiveTargetPercent = 80
	}
	return cfg
}

func (s *Service) GetBlobNodePreset() models.BlobNodePresetConfig {
	s.presetMu.RLock()
	cfg := s.nodePreset
	s.presetMu.RUnlock()
	return models.BlobNodePresetConfig{
		Preset:                     string(cfg.Preset),
		ProfileID:                  cfg.ProfileID,
		StorageProtection:          cfg.StorageProtection,
		Retention:                  cfg.Retention,
		ImageQuotaMB:               cfg.ImageQuotaMB,
		FileQuotaMB:                cfg.FileQuotaMB,
		ImageMaxItemSizeMB:         cfg.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:          cfg.FileMaxItemSizeMB,
		ReplicationMode:            cfg.ReplicationMode,
		RelayEnabled:               cfg.RelayEnabled,
		PublicDiscoveryEnabled:     cfg.PublicDiscoveryEnabled,
		PublicServingEnabled:       cfg.PublicServingEnabled,
		PublicStoreEnabled:         cfg.PublicStoreEnabled,
		PersonalStoreEnabled:       cfg.PersonalStoreEnabled,
		AnnounceEnabled:            cfg.AnnounceEnabled,
		FetchEnabled:               cfg.FetchEnabled,
		RolloutPercent:             cfg.RolloutPercent,
		ServeBandwidthKBps:         cfg.ServeBandwidthKBps,
		ServeBandwidthSoftKBps:     cfg.ServeBandwidthSoftKBps,
		ServeBandwidthHardKBps:     cfg.ServeBandwidthHardKBps,
		ServeMaxConcurrent:         cfg.ServeMaxConcurrent,
		ServeRequestsPerMinPerPeer: cfg.ServeRequestsPerMinPerPeer,
		PublicEphemeralCacheMaxMB:  cfg.PublicEphemeralCacheMaxMB,
		PublicEphemeralCacheTTLMin: cfg.PublicEphemeralCacheTTLMin,
		FetchBandwidthKBps:         cfg.FetchBandwidthKBps,
		HighWatermarkPercent:       cfg.HighWatermarkPercent,
		FullCapPercent:             cfg.FullCapPercent,
		AggressiveTargetPercent:    cfg.AggressiveTargetPercent,
	}
}

func (s *Service) SetBlobNodePreset(rawPreset string) (models.BlobNodePresetConfig, error) {
	preset, err := parseBlobNodePreset(rawPreset)
	if err != nil {
		return models.BlobNodePresetConfig{}, err
	}
	cfg := buildBlobNodePresetConfig(preset)
	if _, err := s.UpdateStoragePolicy(
		cfg.StorageProtection,
		cfg.Retention,
		0,
		0,
		0,
		cfg.ImageQuotaMB,
		cfg.FileQuotaMB,
		cfg.ImageMaxItemSizeMB,
		cfg.FileMaxItemSizeMB,
	); err != nil {
		return models.BlobNodePresetConfig{}, err
	}
	if err := s.SetBlobReplicationMode(cfg.ReplicationMode); err != nil {
		return models.BlobNodePresetConfig{}, err
	}
	if _, err := s.SetBlobFeatureFlags(cfg.AnnounceEnabled, cfg.FetchEnabled, cfg.RolloutPercent); err != nil {
		return models.BlobNodePresetConfig{}, err
	}
	if setter, ok := s.attachmentStore.(interface {
		SetHardCapPolicy(highWatermarkPercent, fullCapPercent, aggressiveTargetPercent int)
	}); ok {
		setter.SetHardCapPolicy(cfg.HighWatermarkPercent, cfg.FullCapPercent, cfg.AggressiveTargetPercent)
	}
	s.configurePublicServingLimits(cfg)
	s.fetchLimiter.SetLimitKBps(cfg.FetchBandwidthKBps)
	s.presetMu.Lock()
	s.nodePreset = cfg
	s.presetMu.Unlock()
	if err := s.syncNodePoliciesFromPreset(cfg); err != nil {
		return models.BlobNodePresetConfig{}, err
	}
	return s.GetBlobNodePreset(), nil
}
