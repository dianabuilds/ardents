package daemonservice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func TestAttachmentMigrationFromLegacyIndexIsIdempotent(t *testing.T) {
	t.Setenv("AIM_ENV", "test")
	t.Setenv("AIM_STORAGE_PASSPHRASE", "migration-secret")

	dataDir := t.TempDir()
	attachmentsDir := filepath.Join(dataDir, "attachments")
	if err := os.MkdirAll(attachmentsDir, 0o700); err != nil {
		t.Fatalf("mkdir attachments dir: %v", err)
	}

	legacyID := "att1_legacy"
	legacyIndex := struct {
		Items map[string]models.AttachmentMeta `json:"items"`
	}{
		Items: map[string]models.AttachmentMeta{
			legacyID: {
				ID:        legacyID,
				Name:      "legacy.txt",
				MimeType:  "text/plain",
				Size:      int64(len("legacy-data")),
				CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
			},
		},
	}
	indexRaw, err := json.Marshal(legacyIndex)
	if err != nil {
		t.Fatalf("marshal legacy index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(attachmentsDir, "index.json"), indexRaw, 0o600); err != nil {
		t.Fatalf("write legacy index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(attachmentsDir, legacyID+".bin"), []byte("legacy-data"), 0o600); err != nil {
		t.Fatalf("write legacy blob: %v", err)
	}

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	svc, err := NewServiceForDaemonWithDataDir(cfg, dataDir)
	if err != nil {
		t.Fatalf("new service with legacy attachments: %v", err)
	}
	meta, data, err := svc.GetAttachment(legacyID)
	if err != nil {
		t.Fatalf("get migrated attachment: %v", err)
	}
	if string(data) != "legacy-data" {
		t.Fatalf("unexpected migrated data: %q", string(data))
	}
	if meta.Class != "file" {
		t.Fatalf("expected class backfilled to file, got %q", meta.Class)
	}
	if meta.PinState != "unpinned" {
		t.Fatalf("expected pin_state backfilled to unpinned, got %q", meta.PinState)
	}
	if meta.LastAccessAt.IsZero() {
		t.Fatal("expected last_access_at backfilled")
	}

	indexAfterFirstBoot, err := os.ReadFile(filepath.Join(attachmentsDir, "index.json"))
	if err != nil {
		t.Fatalf("read index after first boot: %v", err)
	}
	blobAfterFirstBoot, err := os.ReadFile(filepath.Join(attachmentsDir, legacyID+".bin"))
	if err != nil {
		t.Fatalf("read blob after first boot: %v", err)
	}
	if !strings.HasPrefix(string(indexAfterFirstBoot), "AIMENC1\n") {
		t.Fatal("index must be migrated to encrypted format")
	}
	if !strings.HasPrefix(string(blobAfterFirstBoot), "AIMENC1\n") {
		t.Fatal("blob must be migrated to encrypted format")
	}

	svcReloaded, err := NewServiceForDaemonWithDataDir(cfg, dataDir)
	if err != nil {
		t.Fatalf("new service after migration: %v", err)
	}
	metaReloaded, dataReloaded, err := svcReloaded.GetAttachment(legacyID)
	if err != nil {
		t.Fatalf("get attachment after reload: %v", err)
	}
	if string(dataReloaded) != "legacy-data" {
		t.Fatalf("unexpected data after reload: %q", string(dataReloaded))
	}
	if metaReloaded.Class != "file" || metaReloaded.PinState != "unpinned" {
		t.Fatalf("unexpected metadata after reload: %+v", metaReloaded)
	}
}
