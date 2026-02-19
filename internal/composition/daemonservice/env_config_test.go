package daemonservice

import (
	"testing"
	"time"
)

func TestResolveBlobFeatureFlagsFromEnvInvalidValuesFallbackToDefaults(t *testing.T) {
	t.Setenv(blobProviderAnnounceEnv, "maybe")
	t.Setenv(blobProviderFetchEnv, "not_bool")
	t.Setenv(blobProviderRolloutPercentEnv, "oops")

	flags := resolveBlobFeatureFlagsFromEnv()
	if !flags.announceEnabled {
		t.Fatal("announceEnabled must fallback to true")
	}
	if !flags.fetchEnabled {
		t.Fatal("fetchEnabled must fallback to true")
	}
	if flags.rolloutPercent != 100 {
		t.Fatalf("rolloutPercent must fallback to 100, got=%d", flags.rolloutPercent)
	}
}

func TestResolveBlobFeatureFlagsFromEnvClampsRollout(t *testing.T) {
	t.Setenv(blobProviderRolloutPercentEnv, "1000")
	flags := resolveBlobFeatureFlagsFromEnv()
	if flags.rolloutPercent != 100 {
		t.Fatalf("expected rolloutPercent to clamp at 100, got=%d", flags.rolloutPercent)
	}

	t.Setenv(blobProviderRolloutPercentEnv, "-15")
	flags = resolveBlobFeatureFlagsFromEnv()
	if flags.rolloutPercent != 0 {
		t.Fatalf("expected rolloutPercent to clamp at 0, got=%d", flags.rolloutPercent)
	}
}

func TestResolveBlobReplicationModeFromEnvInvalidFallsBackDeterministically(t *testing.T) {
	t.Setenv("AIM_BLOB_REPLICATION_MODE", "invalid-mode")
	if got := resolveBlobReplicationModeFromEnv(); got != blobReplicationModeOnDemand {
		t.Fatalf("expected on_demand fallback, got=%s", got)
	}
}

func TestResolveBlobACLPolicyFromEnvInvalidModeUsesDefaultAndNormalizesAllowlist(t *testing.T) {
	t.Setenv(blobACLModeEnv, "bad_mode")
	t.Setenv(blobACLAllowlistEnv, "  peer1,,peer2, peer1  , ")

	policy := resolveBlobACLPolicyFromEnv()
	if policy.Mode != blobACLModeOwnerContacts {
		t.Fatalf("expected default owner_contacts mode, got=%s", policy.Mode)
	}
	if len(policy.Allowlist) != 2 {
		t.Fatalf("expected deduped allowlist size 2, got=%d", len(policy.Allowlist))
	}
	if _, ok := policy.Allowlist["peer1"]; !ok {
		t.Fatal("expected peer1 in allowlist")
	}
	if _, ok := policy.Allowlist["peer2"]; !ok {
		t.Fatal("expected peer2 in allowlist")
	}
}

func TestNewOutboundMetadataHardeningFromEnvUsesValidatedFallbacks(t *testing.T) {
	t.Setenv("AIM_METADATA_HARDENING", "not_a_bool")
	t.Setenv("AIM_BATCH_WINDOW_MS", "not_a_number")
	t.Setenv("AIM_JITTER_MAX_MS", "9999")

	h := newOutboundMetadataHardeningFromEnv()
	if !h.enabled {
		t.Fatal("expected metadata hardening to fallback to enabled")
	}
	if h.batchWindow != 80*time.Millisecond {
		t.Fatalf("expected default batchWindow 80ms, got=%v", h.batchWindow)
	}
	if h.jitterMax != 600*time.Millisecond {
		t.Fatalf("expected bounded jitterMax 600ms, got=%v", h.jitterMax)
	}
}
