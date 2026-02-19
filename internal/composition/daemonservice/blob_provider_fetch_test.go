package daemonservice

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func newMockConfig() waku.Config {
	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock
	return cfg
}

func newBlobTestService(t *testing.T, cfg waku.Config, label string) *Service {
	t.Helper()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new %s: %v", label, err)
	}
	return svc
}

func createBlobTestIdentity(t *testing.T, svc *Service, label string) {
	t.Helper()
	if _, _, err := svc.CreateIdentity(label + "-pass"); err != nil {
		t.Fatalf("create %s identity: %v", label, err)
	}
}

func startBlobNetworking(t *testing.T, services ...*Service) {
	t.Helper()
	for _, svc := range services {
		if err := svc.StartNetworking(context.Background()); err != nil {
			t.Fatalf("start networking: %v", err)
		}
	}
}

func cleanupBlobNetworking(t *testing.T, services ...*Service) {
	t.Helper()
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, svc := range services {
			_ = svc.StopNetworking(stopCtx)
		}
	})
}

func stopBlobNetworkingNow(t *testing.T, svc *Service, label string) {
	t.Helper()
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := svc.StopNetworking(stopCtx); err != nil {
		cancel()
		t.Fatalf("stop %s networking: %v", label, err)
	}
	cancel()
}

func newStartedBlobTriplet(t *testing.T, cfg waku.Config, registry *blobProviderRegistry) (*Service, *Service, *Service) {
	t.Helper()
	sender := newBlobTestService(t, cfg, "sender")
	provider := newBlobTestService(t, cfg, "provider")
	receiver := newBlobTestService(t, cfg, "receiver")
	useSharedBlobProviders(registry, sender, provider, receiver)
	createBlobTestIdentity(t, sender, "sender")
	createBlobTestIdentity(t, provider, "provider")
	createBlobTestIdentity(t, receiver, "receiver")
	startBlobNetworking(t, sender, provider, receiver)
	cleanupBlobNetworking(t, sender, provider, receiver)
	return sender, provider, receiver
}

func TestGetAttachmentFromPeerProviderWhenSenderOffline(t *testing.T) {
	t.Parallel()
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()

	sender, provider, receiver := newStartedBlobTriplet(t, cfg, registry)

	encoded := base64.StdEncoding.EncodeToString([]byte("shared-content"))
	meta, err := sender.PutAttachment("sample.txt", "text/plain", encoded)
	if err != nil {
		t.Fatalf("sender put attachment: %v", err)
	}

	if _, data, err := provider.GetAttachment(meta.ID); err != nil {
		t.Fatalf("provider on-demand fetch failed: %v", err)
	} else if string(data) != "shared-content" {
		t.Fatalf("unexpected provider fetched content: %q", string(data))
	}

	stopBlobNetworkingNow(t, sender, "sender")

	_, data, err := receiver.GetAttachment(meta.ID)
	if err != nil {
		t.Fatalf("receiver fetch via online provider failed: %v", err)
	}
	if string(data) != "shared-content" {
		t.Fatalf("unexpected receiver content: %q", string(data))
	}
}

func TestGetAttachmentReturnsTemporarilyUnavailableWhenNoProviders(t *testing.T) {
	t.Parallel()
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()

	sender := newBlobTestService(t, cfg, "sender")
	receiver := newBlobTestService(t, cfg, "receiver")
	useSharedBlobProviders(registry, sender, receiver)
	createBlobTestIdentity(t, sender, "sender")
	createBlobTestIdentity(t, receiver, "receiver")
	startBlobNetworking(t, sender, receiver)
	encoded := base64.StdEncoding.EncodeToString([]byte("ephemeral"))
	meta, err := sender.PutAttachment("sample.txt", "text/plain", encoded)
	if err != nil {
		t.Fatalf("sender put attachment: %v", err)
	}
	stopBlobNetworkingNow(t, sender, "sender")
	cleanupBlobNetworking(t, receiver)

	_, _, err = receiver.GetAttachment(meta.ID)
	if !errors.Is(err, contracts.ErrAttachmentTemporarilyUnavailable) {
		t.Fatalf("expected temporary unavailable error, got: %v", err)
	}
}

func TestFetchAttachmentFromProvidersFailover(t *testing.T) {
	t.Parallel()
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()
	svc := newBlobTestService(t, cfg, "service")
	createBlobTestIdentity(t, svc, "receiver")
	useSharedBlobProviders(registry, svc)

	now := time.Now().UTC()
	_ = registry.announceBlob("att1_blob", "aim1bad", time.Minute, func(_ string, _ string) (models.AttachmentMeta, []byte, error) {
		return models.AttachmentMeta{}, nil, errors.New("boom")
	}, now)
	_ = registry.announceBlob("att1_blob", "aim1good", time.Minute, func(_ string, _ string) (models.AttachmentMeta, []byte, error) {
		return models.AttachmentMeta{ID: "att1_blob", Name: "ok.txt", MimeType: "text/plain"}, []byte("ok"), nil
	}, now)

	meta, data, err := svc.fetchAttachmentFromProviders(context.Background(), "att1_blob", svc.localPeerID())
	if err != nil {
		t.Fatalf("expected failover success, got: %v", err)
	}
	if meta.ID != "att1_blob" || string(data) != "ok" {
		t.Fatalf("unexpected failover result: meta=%+v data=%q", meta, string(data))
	}
}

func TestPinnedOnlyReplicationRequiresPinForProviderAnnounce(t *testing.T) {
	t.Setenv("AIM_BLOB_REPLICATION_MODE", "pinned_only")
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()

	sender := newBlobTestService(t, cfg, "sender")
	receiver := newBlobTestService(t, cfg, "receiver")
	useSharedBlobProviders(registry, sender, receiver)
	createBlobTestIdentity(t, sender, "sender")
	createBlobTestIdentity(t, receiver, "receiver")
	startBlobNetworking(t, sender, receiver)
	cleanupBlobNetworking(t, sender, receiver)

	encoded := base64.StdEncoding.EncodeToString([]byte("pinned-only"))
	meta, err := sender.PutAttachment("sample.txt", "text/plain", encoded)
	if err != nil {
		t.Fatalf("sender put attachment: %v", err)
	}
	if providers, _ := sender.ListBlobProviders(meta.ID); len(providers) != 0 {
		t.Fatalf("expected no providers for unpinned blob, got=%d", len(providers))
	}

	stopBlobNetworkingNow(t, sender, "sender")

	_, _, err = receiver.GetAttachment(meta.ID)
	if !errors.Is(err, contracts.ErrAttachmentTemporarilyUnavailable) {
		t.Fatalf("expected temporary unavailable for unpinned blob, got: %v", err)
	}
}

func TestPinnedOnlyReplicationAnnouncesAfterPin(t *testing.T) {
	t.Setenv("AIM_BLOB_REPLICATION_MODE", "pinned_only")
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()

	sender, provider, receiver := newStartedBlobTriplet(t, cfg, registry)

	encoded := base64.StdEncoding.EncodeToString([]byte("pinned-available"))
	meta, err := sender.PutAttachment("sample.txt", "text/plain", encoded)
	if err != nil {
		t.Fatalf("sender put attachment: %v", err)
	}
	updatedMeta, err := sender.PinBlob(meta.ID)
	if err != nil {
		t.Fatalf("pin blob: %v", err)
	}
	if updatedMeta.PinState != "pinned" {
		t.Fatalf("expected pinned state, got %q", updatedMeta.PinState)
	}
	if providers, _ := sender.ListBlobProviders(meta.ID); len(providers) == 0 {
		t.Fatal("expected provider announcement after pin")
	}
	if _, _, err := provider.GetAttachment(meta.ID); err != nil {
		t.Fatalf("provider fetch failed: %v", err)
	}

	stopBlobNetworkingNow(t, sender, "sender")

	_, data, err := receiver.GetAttachment(meta.ID)
	if err != nil {
		t.Fatalf("receiver fetch via pinned provider failed: %v", err)
	}
	if string(data) != "pinned-available" {
		t.Fatalf("unexpected receiver content: %q", string(data))
	}
}

func TestPinnedOnlyReplicationUnpinRemovesProviderAnnounce(t *testing.T) {
	t.Setenv("AIM_BLOB_REPLICATION_MODE", "pinned_only")
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()

	sender := newBlobTestService(t, cfg, "sender")
	useSharedBlobProviders(registry, sender)
	createBlobTestIdentity(t, sender, "sender")
	startBlobNetworking(t, sender)
	cleanupBlobNetworking(t, sender)

	encoded := base64.StdEncoding.EncodeToString([]byte("pin-toggle"))
	meta, err := sender.PutAttachment("sample.txt", "text/plain", encoded)
	if err != nil {
		t.Fatalf("sender put attachment: %v", err)
	}
	if _, err := sender.PinBlob(meta.ID); err != nil {
		t.Fatalf("pin blob: %v", err)
	}
	if providers, _ := sender.ListBlobProviders(meta.ID); len(providers) == 0 {
		t.Fatal("expected providers after pin")
	}
	if _, err := sender.UnpinBlob(meta.ID); err != nil {
		t.Fatalf("unpin blob: %v", err)
	}
	if providers, _ := sender.ListBlobProviders(meta.ID); len(providers) != 0 {
		t.Fatalf("expected providers removed after unpin, got=%d", len(providers))
	}
}

func TestFetchAttachmentFromProvidersRecordsSuccessMetrics(t *testing.T) {
	t.Parallel()
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()
	svc := newBlobTestService(t, cfg, "service")
	createBlobTestIdentity(t, svc, "receiver")
	useSharedBlobProviders(registry, svc)

	now := time.Now().UTC()
	_ = registry.announceBlob("att2_blob", "aim1good", time.Minute, func(_ string, _ string) (models.AttachmentMeta, []byte, error) {
		return models.AttachmentMeta{ID: "att2_blob", Name: "ok.txt", MimeType: "text/plain"}, []byte("ok"), nil
	}, now)

	if _, _, err := svc.fetchAttachmentFromProviders(context.Background(), "att2_blob", svc.localPeerID()); err != nil {
		t.Fatalf("expected fetch success, got: %v", err)
	}

	metrics := svc.GetMetrics()
	if metrics.BlobFetchStats.AttemptsTotal != 1 || metrics.BlobFetchStats.SuccessTotal != 1 {
		t.Fatalf("unexpected success metrics: %+v", metrics.BlobFetchStats)
	}
	if metrics.BlobFetchStats.UnavailableTotal != 0 || metrics.BlobFetchStats.FailureTotal != 0 {
		t.Fatalf("unexpected unavailable/failure metrics: %+v", metrics.BlobFetchStats)
	}
}

func TestFetchAttachmentFromProvidersRecordsUnavailableReason(t *testing.T) {
	t.Parallel()
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()
	svc := newBlobTestService(t, cfg, "service")
	createBlobTestIdentity(t, svc, "receiver")
	useSharedBlobProviders(registry, svc)

	now := time.Now().UTC()
	_ = registry.announceBlob("att3_blob", "aim1bad", time.Minute, func(_ string, _ string) (models.AttachmentMeta, []byte, error) {
		return models.AttachmentMeta{}, nil, errors.New("fetch failed")
	}, now)

	_, _, err := svc.fetchAttachmentFromProviders(context.Background(), "att3_blob", svc.localPeerID())
	if !errors.Is(err, contracts.ErrAttachmentTemporarilyUnavailable) {
		t.Fatalf("expected temporary unavailable error, got: %v", err)
	}

	metrics := svc.GetMetrics()
	if metrics.BlobFetchStats.AttemptsTotal != 1 || metrics.BlobFetchStats.UnavailableTotal != 1 {
		t.Fatalf("unexpected unavailable metrics: %+v", metrics.BlobFetchStats)
	}
	if metrics.BlobFetchStats.UnavailableReasons["providers_failed"] != 1 {
		t.Fatalf("expected providers_failed reason, got: %+v", metrics.BlobFetchStats.UnavailableReasons)
	}
	if metrics.BlobFetchStats.UnavailableRateBps != 10000 {
		t.Fatalf("expected unavailable rate 10000 bps, got: %d", metrics.BlobFetchStats.UnavailableRateBps)
	}
}

func TestFetchAttachmentFromProvidersHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()
	svc := newBlobTestService(t, cfg, "service")
	createBlobTestIdentity(t, svc, "receiver")
	useSharedBlobProviders(registry, svc)

	now := time.Now().UTC()
	_ = registry.announceBlob("att4_blob", "aim1bad", time.Minute, func(_ string, _ string) (models.AttachmentMeta, []byte, error) {
		return models.AttachmentMeta{}, nil, errors.New("fetch failed")
	}, now)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := svc.fetchAttachmentFromProviders(ctx, "att4_blob", svc.localPeerID())
	if !errors.Is(err, contracts.ErrAttachmentTemporarilyUnavailable) {
		t.Fatalf("expected temporary unavailable on canceled context, got: %v", err)
	}

	metrics := svc.GetMetrics()
	if metrics.BlobFetchStats.UnavailableReasons["cancelled"] < 1 {
		t.Fatalf("expected cancelled unavailable reason, got: %+v", metrics.BlobFetchStats.UnavailableReasons)
	}
}

func TestBlobFeatureFlagsDisableProviderAnnounce(t *testing.T) {
	t.Setenv("AIM_BLOB_PROVIDER_ANNOUNCE_ENABLED", "false")
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()
	sender := newBlobTestService(t, cfg, "sender")
	useSharedBlobProviders(registry, sender)
	createBlobTestIdentity(t, sender, "sender")
	startBlobNetworking(t, sender)
	cleanupBlobNetworking(t, sender)

	encoded := base64.StdEncoding.EncodeToString([]byte("disabled-announce"))
	meta, err := sender.PutAttachment("sample.txt", "text/plain", encoded)
	if err != nil {
		t.Fatalf("sender put attachment: %v", err)
	}
	if providers, _ := sender.ListBlobProviders(meta.ID); len(providers) != 0 {
		t.Fatalf("expected no providers when announce is disabled, got=%d", len(providers))
	}
}

func TestBlobFeatureFlagsDisableProviderFetchKeepsLocalAccess(t *testing.T) {
	t.Setenv("AIM_BLOB_PROVIDER_FETCH_ENABLED", "false")
	registry := newBlobProviderRegistry()

	cfg := newMockConfig()
	sender := newBlobTestService(t, cfg, "sender")
	receiver := newBlobTestService(t, cfg, "receiver")
	useSharedBlobProviders(registry, sender, receiver)
	createBlobTestIdentity(t, sender, "sender")
	createBlobTestIdentity(t, receiver, "receiver")
	startBlobNetworking(t, sender, receiver)
	cleanupBlobNetworking(t, sender, receiver)

	localPayload := base64.StdEncoding.EncodeToString([]byte("local-only"))
	localMeta, err := receiver.PutAttachment("local.txt", "text/plain", localPayload)
	if err != nil {
		t.Fatalf("receiver put local attachment: %v", err)
	}
	if _, data, err := receiver.GetAttachment(localMeta.ID); err != nil {
		t.Fatalf("expected local access during rollback mode, got: %v", err)
	} else if string(data) != "local-only" {
		t.Fatalf("unexpected local payload: %q", string(data))
	}

	remotePayload := base64.StdEncoding.EncodeToString([]byte("remote-only"))
	remoteMeta, err := sender.PutAttachment("remote.txt", "text/plain", remotePayload)
	if err != nil {
		t.Fatalf("sender put remote attachment: %v", err)
	}
	_, _, err = receiver.GetAttachment(remoteMeta.ID)
	if !errors.Is(err, storage.ErrAttachmentNotFound) {
		t.Fatalf("expected local-not-found with provider fetch disabled, got: %v", err)
	}
}
