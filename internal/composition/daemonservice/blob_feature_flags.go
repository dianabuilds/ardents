package daemonservice

import (
	"hash/fnv"
	"strings"

	"aim-chat/go-backend/pkg/models"
)

const (
	blobProviderAnnounceEnv       = "AIM_BLOB_PROVIDER_ANNOUNCE_ENABLED"
	blobProviderFetchEnv          = "AIM_BLOB_PROVIDER_FETCH_ENABLED"
	blobProviderRolloutPercentEnv = "AIM_BLOB_PROVIDER_ROLLOUT_PERCENT"
)

type blobFeatureFlags struct {
	announceEnabled bool
	fetchEnabled    bool
	rolloutPercent  int
}

func resolveBlobFeatureFlagsFromEnv() blobFeatureFlags {
	flags := blobFeatureFlags{
		announceEnabled: true,
		fetchEnabled:    true,
		rolloutPercent:  100,
	}
	flags.announceEnabled = envBoolWithFallback(blobProviderAnnounceEnv, flags.announceEnabled)
	flags.fetchEnabled = envBoolWithFallback(blobProviderFetchEnv, flags.fetchEnabled)
	flags.rolloutPercent = clampRolloutPercent(envIntWithFallback(blobProviderRolloutPercentEnv, flags.rolloutPercent))
	return flags
}

func (f blobFeatureFlags) toModel() models.BlobFeatureFlags {
	return models.BlobFeatureFlags{
		AnnounceEnabled: f.announceEnabled,
		FetchEnabled:    f.fetchEnabled,
		RolloutPercent:  f.rolloutPercent,
	}
}

func (f blobFeatureFlags) allowsPeer(peerID string) bool {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return false
	}
	if f.rolloutPercent <= 0 {
		return false
	}
	if f.rolloutPercent >= 100 {
		return true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(peerID))
	bucket := int(h.Sum32() % 100)
	return bucket < f.rolloutPercent
}

func clampRolloutPercent(raw int) int {
	if raw < 0 {
		return 0
	}
	if raw > 100 {
		return 100
	}
	return raw
}
