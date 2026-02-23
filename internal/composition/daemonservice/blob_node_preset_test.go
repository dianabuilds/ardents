package daemonservice

import (
	"encoding/base64"
	"errors"
	"testing"

	"aim-chat/go-backend/internal/domains/contracts"
)

func newBlobPresetPair(t *testing.T) (*Service, *Service) {
	t.Helper()
	registry := newBlobProviderRegistry()
	cfg := newMockConfig()
	sender, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	receiver, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new receiver: %v", err)
	}
	useSharedBlobProviders(registry, sender, receiver)
	if _, _, err := sender.CreateIdentity("sender-pass"); err != nil {
		t.Fatalf("create sender identity: %v", err)
	}
	if _, _, err := receiver.CreateIdentity("receiver-pass"); err != nil {
		t.Fatalf("create receiver identity: %v", err)
	}
	startBlobNetworking(t, sender, receiver)
	cleanupBlobNetworking(t, sender, receiver)
	return sender, receiver
}

func TestSetBlobNodePresetAppliesRuntimePolicy(t *testing.T) {
	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, _, err := svc.CreateIdentity("pass"); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	applied, err := svc.SetBlobNodePreset("pin")
	if err != nil {
		t.Fatalf("set blob node preset: %v", err)
	}
	if applied.Preset != "pin" {
		t.Fatalf("unexpected preset: %+v", applied)
	}
	if mode := svc.GetBlobReplicationMode(); mode != "pinned_only" {
		t.Fatalf("unexpected replication mode: %s", mode)
	}
	flags := svc.GetBlobFeatureFlags()
	if !flags.AnnounceEnabled || !flags.FetchEnabled {
		t.Fatalf("unexpected blob flags: %+v", flags)
	}
	policy, err := svc.GetStoragePolicy()
	if err != nil {
		t.Fatalf("get storage policy: %v", err)
	}
	if policy.ImageQuotaMB != applied.ImageQuotaMB || policy.FileQuotaMB != applied.FileQuotaMB {
		t.Fatalf("preset quotas were not applied: policy=%+v preset=%+v", policy, applied)
	}
}

func TestDefaultBlobNodePresetIsNetworkAssist(t *testing.T) {
	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	got := svc.GetBlobNodePreset()
	if got.Preset != "assist" {
		t.Fatalf("expected default preset assist, got %q", got.Preset)
	}
	if got.ProfileID != "network_assist_default" {
		t.Fatalf("expected profile_id network_assist_default, got %q", got.ProfileID)
	}
	if !got.RelayEnabled || !got.PublicDiscoveryEnabled || !got.PublicServingEnabled || !got.PersonalStoreEnabled {
		t.Fatalf("unexpected default network assist toggles: %+v", got)
	}
	if got.PublicStoreEnabled {
		t.Fatalf("public store must be disabled by default: %+v", got)
	}
}

func TestBlobServeBandwidthCapLimitsProviderFetch(t *testing.T) {
	sender, receiver := newBlobPresetPair(t)
	if _, err := sender.SetBlobNodePreset("lite"); err != nil {
		t.Fatalf("set sender preset: %v", err)
	}
	// Keep lite bandwidth caps but allow provider announce/fetch for this scenario.
	if err := sender.SetBlobReplicationMode("on_demand"); err != nil {
		t.Fatalf("set replication mode: %v", err)
	}
	if _, err := sender.SetBlobFeatureFlags(true, true, 100); err != nil {
		t.Fatalf("set feature flags: %v", err)
	}

	oversized := make([]byte, 600*1024) // > lite hard cap (512 KB/s burst)
	meta, err := sender.PutAttachment("large.bin", "application/octet-stream", base64.StdEncoding.EncodeToString(oversized))
	if err != nil {
		t.Fatalf("put attachment: %v", err)
	}
	_, _, err = receiver.GetAttachment(meta.ID)
	if !errors.Is(err, contracts.ErrAttachmentTemporarilyUnavailable) {
		t.Fatalf("expected temporary unavailable due bandwidth cap, got: %v", err)
	}
}
