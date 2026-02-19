package daemonservice

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func newStoragePolicyTestService(t *testing.T) *Service {
	t.Helper()
	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func seedExpiredMessageAndAttachment(t *testing.T, svc *Service) models.AttachmentMeta {
	t.Helper()
	oldTS := time.Now().UTC().Add(-4 * time.Second)
	if err := svc.messageStore.SaveMessage(models.Message{
		ID:        "msg_old",
		ContactID: "aim1_contact",
		Content:   []byte("old"),
		Timestamp: oldTS,
		Direction: "in",
		Status:    "delivered",
	}); err != nil {
		t.Fatalf("save old message: %v", err)
	}
	att, err := svc.attachmentStore.Put("old.txt", "text/plain", []byte("old"))
	if err != nil {
		t.Fatalf("put old attachment: %v", err)
	}
	return att
}

func TestUpdateStoragePolicyZeroRetentionDisablesPersistence(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	dataDir := t.TempDir()
	svc, err := NewServiceForDaemonWithDataDir(cfg, dataDir)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	msg := models.Message{
		ID:        "msg_before_zero",
		ContactID: "aim1_contact",
		Content:   []byte("secret"),
		Timestamp: time.Now().UTC(),
		Direction: "out",
		Status:    "sent",
	}
	if err := svc.messageStore.SaveMessage(msg); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if _, err := svc.sessionManager.InitSession("aim1_local", "aim1_contact", make([]byte, 32)); err != nil {
		t.Fatalf("init session: %v", err)
	}
	if _, err := svc.attachmentStore.Put("a.txt", "text/plain", []byte("payload")); err != nil {
		t.Fatalf("put attachment: %v", err)
	}

	policy, err := svc.UpdateStoragePolicy("protected", "zero_retention", 0, 0, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	if policy.ContentRetentionMode != privacydomain.RetentionZeroRetention {
		t.Fatalf("unexpected retention mode: %q", policy.ContentRetentionMode)
	}

	msgAfter := models.Message{
		ID:        "msg_after_zero",
		ContactID: "aim1_contact",
		Content:   []byte("runtime"),
		Timestamp: time.Now().UTC(),
		Direction: "out",
		Status:    "sent",
	}
	if err := svc.messageStore.SaveMessage(msgAfter); err != nil {
		t.Fatalf("save message in zero retention: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "messages.json")); !os.IsNotExist(err) {
		t.Fatalf("messages.json must not exist in zero retention mode")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "sessions.json")); !os.IsNotExist(err) {
		t.Fatalf("sessions.json must not exist in zero retention mode")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "attachments")); !os.IsNotExist(err) {
		t.Fatalf("attachments directory must not exist in zero retention mode")
	}
}

func TestExportBackupBlockedInZeroRetention(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := svc.UpdateStoragePolicy("standard", "zero_retention", 0, 0, 0, 0, 0, 0, 0); err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	_, err = svc.ExportBackup("I_UNDERSTAND_BACKUP_RISK", "pass")
	if !errors.Is(err, ErrBackupDisabledByRetentionPolicy) {
		t.Fatalf("expected ErrBackupDisabledByRetentionPolicy, got: %v", err)
	}
}

func TestEnforceRetentionPoliciesEphemeralPurgesExpiredContent(t *testing.T) {
	t.Parallel()

	svc := newStoragePolicyTestService(t)
	att := seedExpiredMessageAndAttachment(t, svc)

	if _, err := svc.UpdateStoragePolicy("standard", "ephemeral", 1, 1, 1, 0, 0, 0, 0); err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	svc.enforceRetentionPolicies(att.CreatedAt.Add(2 * time.Second))

	msgs, _ := svc.messageStore.Snapshot()
	if len(msgs) != 0 {
		t.Fatalf("expected expired messages to be purged, got %d", len(msgs))
	}
	if _, _, err := svc.attachmentStore.Get(att.ID); err == nil {
		t.Fatalf("expected expired attachment to be purged")
	}
}

func TestEnforceRetentionPoliciesEphemeralKeepsFilesWhenFileTTLDisabled(t *testing.T) {
	t.Parallel()

	svc := newStoragePolicyTestService(t)
	att := seedExpiredMessageAndAttachment(t, svc)

	// Explicitly disable file TTL in ephemeral mode while keeping message TTL enabled.
	if _, err := svc.UpdateStoragePolicy("standard", "ephemeral", 1, 0, 0, 0, 0, 0, 0); err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	svc.enforceRetentionPolicies(att.CreatedAt.Add(2 * time.Second))

	msgs, _ := svc.messageStore.Snapshot()
	if len(msgs) != 0 {
		t.Fatalf("expected expired messages to be purged, got %d", len(msgs))
	}
	if _, _, err := svc.attachmentStore.Get(att.ID); err != nil {
		t.Fatalf("expected attachment to remain when file ttl is disabled, got: %v", err)
	}
}

func TestEnforceRetentionPoliciesEphemeralPurgesImagesAndKeepsFilesIndependently(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	dataDir := t.TempDir()
	svc, err := NewServiceForDaemonWithDataDir(cfg, dataDir)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	imageData := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0x99, 0x63, 0x60, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x01, 0xf4, 0x71, 0x64, 0xa6, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
	imageAtt, err := svc.attachmentStore.Put("old.png", "image/png", imageData)
	if err != nil {
		t.Fatalf("put image attachment: %v", err)
	}
	fileAtt, err := svc.attachmentStore.Put("old.txt", "text/plain", []byte("old"))
	if err != nil {
		t.Fatalf("put file attachment: %v", err)
	}

	if _, err := svc.UpdateStoragePolicy("standard", "ephemeral", 1, 1, 0, 0, 0, 0, 0); err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	retentionNow := imageAtt.CreatedAt.Add(2 * time.Second)
	if fileAtt.CreatedAt.After(imageAtt.CreatedAt) {
		retentionNow = fileAtt.CreatedAt.Add(2 * time.Second)
	}
	svc.enforceRetentionPolicies(retentionNow)

	if _, _, err := svc.attachmentStore.Get(imageAtt.ID); err == nil {
		t.Fatalf("expected image attachment to be purged by image ttl")
	}
	if _, _, err := svc.attachmentStore.Get(fileAtt.ID); err != nil {
		t.Fatalf("expected file attachment to remain when file ttl is disabled, got: %v", err)
	}
}

func TestEnforceRetentionPoliciesRecordsGCEvictionsByClass(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.attachmentStore.Put("old.txt", "text/plain", []byte("old")); err != nil {
		t.Fatalf("put attachment: %v", err)
	}
	if _, err := svc.UpdateStoragePolicy("standard", "ephemeral", 1, 0, 1, 0, 0, 0, 0); err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	svc.enforceRetentionPolicies(now.Add(2 * time.Second))

	metrics := svc.GetMetrics()
	if metrics.GCEvictionCountByClass["file"] < 1 {
		t.Fatalf("expected gc eviction metric for file class, got %+v", metrics.GCEvictionCountByClass)
	}
}

func TestRunAttachmentGCDryRunDoesNotMutateState(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.attachmentStore.Put("a.txt", "text/plain", make([]byte, 700*1024)); err != nil {
		t.Fatalf("put first attachment: %v", err)
	}
	second, err := svc.attachmentStore.Put("b.txt", "text/plain", make([]byte, 700*1024))
	if err != nil {
		t.Fatalf("put second attachment: %v", err)
	}
	if _, err := svc.UpdateStoragePolicy("standard", "persistent", 0, 0, 0, 0, 1, 0, 0); err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	report, err := svc.RunAttachmentGCDryRun(time.Now().UTC())
	if err != nil {
		t.Fatalf("dry-run gc failed: %v", err)
	}
	if !report.DryRun {
		t.Fatalf("expected dry run report")
	}
	if report.DeletedByCause["lru"] < 1 {
		t.Fatalf("expected lru candidates in dry-run report, got %+v", report)
	}
	if _, _, err := svc.attachmentStore.Get(second.ID); err != nil {
		t.Fatalf("dry-run must not delete attachments, got: %v", err)
	}
}
