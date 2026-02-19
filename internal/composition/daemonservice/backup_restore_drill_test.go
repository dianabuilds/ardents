package daemonservice

import (
	"testing"
	"time"

	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

const (
	backupDrillExportSLAMax  = 5 * time.Second
	backupDrillRestoreSLAMax = 5 * time.Second
)

func TestBackupRestoreDrillSLA(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	identity, _, err := svc.CreateIdentity("seed-pass")
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if err := svc.AddContact("aim1contact0001", "Alice"); err != nil {
		t.Fatalf("add contact: %v", err)
	}
	message := models.Message{
		ID:        "msg-backup-1",
		ContactID: "aim1contact0001",
		Content:   []byte("hello"),
		Timestamp: time.Now().UTC(),
		Direction: "out",
		Status:    "sent",
	}
	if err := svc.messageStore.SaveMessage(message); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if _, err := svc.sessionManager.InitSession(identity.ID, "aim1contact0001", make([]byte, 32)); err != nil {
		t.Fatalf("init session: %v", err)
	}

	exportStarted := time.Now()
	blob, err := svc.ExportBackup("I_UNDERSTAND_BACKUP_RISK", "backup-passphrase")
	if err != nil {
		t.Fatalf("export backup: %v", err)
	}
	exportDuration := time.Since(exportStarted)

	if _, err := svc.WipeData(DataWipeConsentToken); err != nil {
		t.Fatalf("wipe data: %v", err)
	}

	restoreStarted := time.Now()
	restoredIdentity, err := svc.RestoreBackup("I_UNDERSTAND_BACKUP_RISK", "backup-passphrase", blob)
	if err != nil {
		t.Fatalf("restore backup: %v", err)
	}
	restoreDuration := time.Since(restoreStarted)

	if restoredIdentity.ID != identity.ID {
		t.Fatalf("restored identity mismatch: got=%q want=%q", restoredIdentity.ID, identity.ID)
	}
	contacts, err := svc.GetContacts()
	if err != nil {
		t.Fatalf("get contacts: %v", err)
	}
	if len(contacts) != 1 || contacts[0].ID != "aim1contact0001" {
		t.Fatalf("unexpected restored contacts: %#v", contacts)
	}
	messages, pending := svc.messageStore.Snapshot()
	if len(messages) != 1 || len(pending) != 0 {
		t.Fatalf("unexpected restored messages: messages=%d pending=%d", len(messages), len(pending))
	}
	sessions, err := svc.sessionManager.Snapshot()
	if err != nil {
		t.Fatalf("snapshot sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("unexpected restored sessions count: %d", len(sessions))
	}

	if exportDuration > backupDrillExportSLAMax {
		t.Fatalf("backup export SLA exceeded: got=%s max=%s", exportDuration, backupDrillExportSLAMax)
	}
	if restoreDuration > backupDrillRestoreSLAMax {
		t.Fatalf("backup restore SLA exceeded: got=%s max=%s", restoreDuration, backupDrillRestoreSLAMax)
	}
	t.Logf(
		"backup_drill export_ms=%d restore_ms=%d export_sla_ms=%d restore_sla_ms=%d",
		exportDuration.Milliseconds(),
		restoreDuration.Milliseconds(),
		backupDrillExportSLAMax.Milliseconds(),
		backupDrillRestoreSLAMax.Milliseconds(),
	)
}
