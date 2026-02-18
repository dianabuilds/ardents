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

	policy, err := svc.UpdateStoragePolicy("protected", "zero_retention", 0, 0)
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

	if _, err := svc.UpdateStoragePolicy("standard", "zero_retention", 0, 0); err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	_, err = svc.ExportBackup("I_UNDERSTAND_BACKUP_RISK", "pass")
	if !errors.Is(err, ErrBackupDisabledByRetentionPolicy) {
		t.Fatalf("expected ErrBackupDisabledByRetentionPolicy, got: %v", err)
	}
}

func TestEnforceRetentionPoliciesEphemeralPurgesExpiredContent(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	dataDir := t.TempDir()
	svc, err := NewServiceForDaemonWithDataDir(cfg, dataDir)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

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
	time.Sleep(1200 * time.Millisecond)

	if _, err := svc.UpdateStoragePolicy("standard", "ephemeral", 1, 1); err != nil {
		t.Fatalf("update storage policy: %v", err)
	}
	svc.enforceRetentionPolicies(time.Now().UTC())

	msgs, _ := svc.messageStore.Snapshot()
	if len(msgs) != 0 {
		t.Fatalf("expected expired messages to be purged, got %d", len(msgs))
	}
	if _, _, err := svc.attachmentStore.Get(att.ID); err == nil {
		t.Fatalf("expected expired attachment to be purged")
	}
}
