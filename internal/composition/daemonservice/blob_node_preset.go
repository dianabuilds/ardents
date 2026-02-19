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
	blobNodePresetLite   blobNodePreset = "lite"
	blobNodePresetCache  blobNodePreset = "cache"
	blobNodePresetPin    blobNodePreset = "pin"
)

type blobNodePresetConfig struct {
	Preset                  blobNodePreset
	StorageProtection       string
	Retention               string
	ImageQuotaMB            int
	FileQuotaMB             int
	ImageMaxItemSizeMB      int
	FileMaxItemSizeMB       int
	ReplicationMode         string
	AnnounceEnabled         bool
	FetchEnabled            bool
	RolloutPercent          int
	ServeBandwidthKBps      int
	FetchBandwidthKBps      int
	HighWatermarkPercent    int
	FullCapPercent          int
	AggressiveTargetPercent int
}

var errInvalidBlobNodePreset = errors.New("invalid blob node preset")

func defaultBlobNodePresetConfig() blobNodePresetConfig {
	return blobNodePresetConfig{
		Preset:                  blobNodePresetCustom,
		StorageProtection:       string(privacydomain.StorageProtectionStandard),
		Retention:               string(privacydomain.RetentionPersistent),
		ImageQuotaMB:            0,
		FileQuotaMB:             0,
		ImageMaxItemSizeMB:      0,
		FileMaxItemSizeMB:       0,
		ReplicationMode:         string(blobReplicationModeOnDemand),
		AnnounceEnabled:         true,
		FetchEnabled:            true,
		RolloutPercent:          100,
		ServeBandwidthKBps:      0,
		FetchBandwidthKBps:      0,
		HighWatermarkPercent:    90,
		FullCapPercent:          100,
		AggressiveTargetPercent: 75,
	}
}

func parseBlobNodePreset(raw string) (blobNodePreset, error) {
	switch blobNodePreset(strings.ToLower(strings.TrimSpace(raw))) {
	case blobNodePresetCustom:
		return blobNodePresetCustom, nil
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
	cfg := defaultBlobNodePresetConfig()
	cfg.Preset = preset
	switch preset {
	case blobNodePresetLite:
		cfg.ImageQuotaMB = 256
		cfg.FileQuotaMB = 512
		cfg.ImageMaxItemSizeMB = 8
		cfg.FileMaxItemSizeMB = 16
		cfg.ReplicationMode = string(blobReplicationModeNone)
		cfg.AnnounceEnabled = false
		cfg.FetchEnabled = true
		cfg.ServeBandwidthKBps = 256
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
		Preset:                  string(cfg.Preset),
		StorageProtection:       cfg.StorageProtection,
		Retention:               cfg.Retention,
		ImageQuotaMB:            cfg.ImageQuotaMB,
		FileQuotaMB:             cfg.FileQuotaMB,
		ImageMaxItemSizeMB:      cfg.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:       cfg.FileMaxItemSizeMB,
		ReplicationMode:         cfg.ReplicationMode,
		AnnounceEnabled:         cfg.AnnounceEnabled,
		FetchEnabled:            cfg.FetchEnabled,
		RolloutPercent:          cfg.RolloutPercent,
		ServeBandwidthKBps:      cfg.ServeBandwidthKBps,
		FetchBandwidthKBps:      cfg.FetchBandwidthKBps,
		HighWatermarkPercent:    cfg.HighWatermarkPercent,
		FullCapPercent:          cfg.FullCapPercent,
		AggressiveTargetPercent: cfg.AggressiveTargetPercent,
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
	s.serveLimiter.SetLimitKBps(cfg.ServeBandwidthKBps)
	s.fetchLimiter.SetLimitKBps(cfg.FetchBandwidthKBps)
	s.presetMu.Lock()
	s.nodePreset = cfg
	s.presetMu.Unlock()
	return s.GetBlobNodePreset(), nil
}
