package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/pkg/models"
)

func TestAttachmentStorePutRollbackOnPersistError(t *testing.T) {
	dir := t.TempDir()
	indexAsDir := filepath.Join(dir, "index-as-dir")
	if err := os.MkdirAll(indexAsDir, 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	store := &AttachmentStore{
		dir:       dir,
		indexPath: indexAsDir, // directory path forces os.WriteFile error
		items:     make(map[string]models.AttachmentMeta),
		blobs:     make(map[string][]byte),
		persist:   true,
	}

	if _, err := store.Put("a.txt", "text/plain", []byte("hello")); err == nil {
		t.Fatal("expected put error")
	}
	if len(store.items) != 0 {
		t.Fatalf("items map must stay unchanged after persist failure, got %d", len(store.items))
	}
	files, err := filepath.Glob(filepath.Join(dir, "att1_*.bin"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("attachment file should be cleaned up on persist failure, found %d", len(files))
	}
}

func TestAttachmentStoreCreatesPrivateDir(t *testing.T) {
	baseDir := t.TempDir()
	attachmentsDir := filepath.Join(baseDir, "secure", "attachments")
	store, err := NewAttachmentStore(attachmentsDir)
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}
	if _, err := store.Put("a.txt", "text/plain", []byte("hello")); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	info, err := os.Stat(attachmentsDir)
	if err != nil {
		t.Fatalf("stat dir failed: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("expected dir perm 0700, got %04o", info.Mode().Perm())
	}
}

func TestAttachmentStoreWithSecretEncryptsAndReadsAttachment(t *testing.T) {
	baseDir := t.TempDir()
	attachmentsDir := filepath.Join(baseDir, "secure", "attachments")
	store, err := NewAttachmentStoreWithSecret(attachmentsDir, "test-secret")
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}
	meta, err := store.Put("a.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(attachmentsDir, meta.ID+".bin"))
	if err != nil {
		t.Fatalf("read raw attachment failed: %v", err)
	}
	if strings.Contains(string(raw), "hello") {
		t.Fatal("attachment blob must not be stored in plaintext when secret is set")
	}
	if !strings.HasPrefix(string(raw), "AIMENC1\n") {
		t.Fatal("attachment blob must be stored in encrypted envelope format")
	}

	_, data, err := store.Get(meta.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected attachment payload: %q", string(data))
	}
}

func TestAttachmentStoreWithSecretMigratesLegacyPlaintext(t *testing.T) {
	baseDir := t.TempDir()
	attachmentsDir := filepath.Join(baseDir, "attachments")
	legacy, err := NewAttachmentStore(attachmentsDir)
	if err != nil {
		t.Fatalf("new legacy store failed: %v", err)
	}
	meta, err := legacy.Put("a.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatalf("legacy put failed: %v", err)
	}

	store, err := NewAttachmentStoreWithSecret(attachmentsDir, "test-secret")
	if err != nil {
		t.Fatalf("new protected store failed: %v", err)
	}
	_, data, err := store.Get(meta.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected attachment payload after migration: %q", string(data))
	}

	rawIndex, err := os.ReadFile(filepath.Join(attachmentsDir, "index.json"))
	if err != nil {
		t.Fatalf("read index failed: %v", err)
	}
	if !strings.HasPrefix(string(rawIndex), "AIMENC1\n") {
		t.Fatal("index must be migrated to encrypted format")
	}
	rawBlob, err := os.ReadFile(filepath.Join(attachmentsDir, meta.ID+".bin"))
	if err != nil {
		t.Fatalf("read attachment failed: %v", err)
	}
	if !strings.HasPrefix(string(rawBlob), "AIMENC1\n") {
		t.Fatal("attachment blob must be migrated to encrypted format")
	}
}

func TestAttachmentStoreWithSecretFailsOnTamperedCiphertext(t *testing.T) {
	baseDir := t.TempDir()
	attachmentsDir := filepath.Join(baseDir, "attachments")
	store, err := NewAttachmentStoreWithSecret(attachmentsDir, "test-secret")
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	meta, err := store.Put("a.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}
	path := filepath.Join(attachmentsDir, meta.ID+".bin")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read blob failed: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty encrypted blob")
	}
	raw[len(raw)-1] ^= 0x01
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write tampered blob failed: %v", err)
	}

	_, _, err = store.Get(meta.ID)
	if !errors.Is(err, securestore.ErrAuthFailed) && !errors.Is(err, securestore.ErrInvalid) {
		t.Fatalf("expected securestore auth/invalid error, got: %v", err)
	}
}

func TestAttachmentStoreRollforwardWritesSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	legacy := struct {
		Items map[string]models.AttachmentMeta `json:"items"`
	}{
		Items: map[string]models.AttachmentMeta{
			"att1_legacy": {
				ID:           "att1_legacy",
				Name:         "legacy.txt",
				MimeType:     "text/plain",
				Class:        "file",
				Size:         3,
				PinState:     "unpinned",
				CreatedAt:    time.Now().UTC(),
				LastAccessAt: time.Now().UTC(),
			},
		},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy index failed: %v", err)
	}
	if err := os.WriteFile(indexPath, raw, 0o600); err != nil {
		t.Fatalf("write legacy index failed: %v", err)
	}

	if _, err := NewAttachmentStore(dir); err != nil {
		t.Fatalf("open store failed: %v", err)
	}
	updatedRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read updated index failed: %v", err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(updatedRaw, &snapshot); err != nil {
		t.Fatalf("decode updated index failed: %v", err)
	}
	version, ok := snapshot["schema_version"].(float64)
	if !ok {
		t.Fatal("expected schema_version field in migrated snapshot")
	}
	if int(version) != attachmentIndexSchemaVersion {
		t.Fatalf("unexpected schema version: got=%d want=%d", int(version), attachmentIndexSchemaVersion)
	}
}

func TestAttachmentStoreFailsOnFutureSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	payload := map[string]any{
		"schema_version": attachmentIndexSchemaVersion + 1,
		"items":          map[string]any{},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if err := os.WriteFile(indexPath, raw, 0o600); err != nil {
		t.Fatalf("write payload failed: %v", err)
	}

	_, err = NewAttachmentStore(dir)
	if !errors.Is(err, ErrUnsupportedStorageSchema) {
		t.Fatalf("expected ErrUnsupportedStorageSchema, got %v", err)
	}
}

func TestAttachmentStorePurgeOlderThan(t *testing.T) {
	baseDir := t.TempDir()
	attachmentsDir := filepath.Join(baseDir, "attachments")
	store, err := NewAttachmentStoreWithSecret(attachmentsDir, "test-secret")
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	oldMeta, err := store.Put("old.txt", "text/plain", []byte("old"))
	if err != nil {
		t.Fatalf("put old failed: %v", err)
	}
	newMeta, err := store.Put("new.txt", "text/plain", []byte("new"))
	if err != nil {
		t.Fatalf("put new failed: %v", err)
	}

	store.mu.Lock()
	old := store.items[oldMeta.ID]
	old.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
	store.items[oldMeta.ID] = old
	store.mu.Unlock()

	deleted, err := store.PurgeOlderThan(time.Now().UTC().Add(-30 * time.Minute))
	if err != nil {
		t.Fatalf("purge failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one deleted attachment, got %d", deleted)
	}
	if _, _, err := store.Get(oldMeta.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("old attachment must be removed, got: %v", err)
	}
	if _, _, err := store.Get(newMeta.ID); err != nil {
		t.Fatalf("new attachment must stay, got: %v", err)
	}
}

func TestAttachmentStorePutSetsAttachmentClass(t *testing.T) {
	store, err := NewAttachmentStore("")
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}

	txt, err := store.Put("a.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatalf("put text failed: %v", err)
	}
	if txt.Class != "file" {
		t.Fatalf("expected text attachment class=file, got %q", txt.Class)
	}

	img, err := store.Put("a.png", "image/png", []byte("png-data"))
	if err != nil {
		t.Fatalf("put image failed: %v", err)
	}
	if img.Class != "image" {
		t.Fatalf("expected image attachment class=image, got %q", img.Class)
	}
}

func TestAttachmentStoreLoadBackfillsAttachmentClass(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	legacy := struct {
		Items map[string]models.AttachmentMeta `json:"items"`
	}{
		Items: map[string]models.AttachmentMeta{
			"att1_old": {
				ID:        "att1_old",
				Name:      "old.png",
				MimeType:  "image/png",
				Size:      3,
				CreatedAt: time.Now().UTC(),
			},
		},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy index failed: %v", err)
	}
	if err := os.WriteFile(indexPath, raw, 0o600); err != nil {
		t.Fatalf("write legacy index failed: %v", err)
	}

	store, err := NewAttachmentStore(dir)
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}
	store.mu.RLock()
	meta, ok := store.items["att1_old"]
	store.mu.RUnlock()
	if !ok {
		t.Fatalf("expected legacy item to be loaded")
	}
	if meta.Class != "image" {
		t.Fatalf("expected legacy class backfill to image, got %q", meta.Class)
	}
	if meta.PinState != "unpinned" {
		t.Fatalf("expected default pin state unpinned, got %q", meta.PinState)
	}
	if meta.LastAccessAt.IsZero() {
		t.Fatal("expected last_access_at to be backfilled")
	}
}

func TestAttachmentStorePurgeOlderThanByClass(t *testing.T) {
	store, err := NewAttachmentStore("")
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}
	imageMeta, err := store.Put("a.png", "image/png", []byte("img"))
	if err != nil {
		t.Fatalf("put image failed: %v", err)
	}
	fileMeta, err := store.Put("a.txt", "text/plain", []byte("file"))
	if err != nil {
		t.Fatalf("put file failed: %v", err)
	}

	store.mu.Lock()
	image := store.items[imageMeta.ID]
	image.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
	store.items[imageMeta.ID] = image
	file := store.items[fileMeta.ID]
	file.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
	store.items[fileMeta.ID] = file
	store.mu.Unlock()

	deleted, err := store.PurgeOlderThanByClass("image", time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("purge by class failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one deleted image, got %d", deleted)
	}
	store.mu.RLock()
	_, imageExists := store.items[imageMeta.ID]
	_, fileExists := store.items[fileMeta.ID]
	store.mu.RUnlock()
	if imageExists {
		t.Fatal("expected image attachment removed")
	}
	if !fileExists {
		t.Fatal("expected file attachment to stay")
	}
}

func TestAttachmentStoreEnforcesClassPolicies(t *testing.T) {
	store, err := NewAttachmentStore("")
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}
	store.SetClassPolicies(1, 1, 1, 1)

	if _, err := store.Put("img.png", "image/png", make([]byte, 2*1024*1024)); err == nil {
		t.Fatal("expected image max item size policy error")
	}
	if _, err := store.Put("file.bin", "application/octet-stream", make([]byte, 2*1024*1024)); err == nil {
		t.Fatal("expected file max item size policy error")
	}

	if _, err := store.Put("f1.bin", "application/octet-stream", make([]byte, 700*1024)); err != nil {
		t.Fatalf("put file #1 failed: %v", err)
	}
	if _, err := store.Put("f2.bin", "application/octet-stream", make([]byte, 700*1024)); err != nil {
		t.Fatalf("put file #2 failed: %v", err)
	}

	usage := store.UsageByClass()
	if usage["file"] <= 0 || usage["file"] > int64(1024*1024) {
		t.Fatalf("expected file usage to be tracked, got %d", usage["file"])
	}
}

func TestAttachmentStoreRunGCEvictsExpiredThenLRUAndSkipsPinned(t *testing.T) {
	store, err := NewAttachmentStore("")
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}

	pinned, err := store.Put("pinned.bin", "application/octet-stream", make([]byte, 300*1024))
	if err != nil {
		t.Fatalf("put pinned failed: %v", err)
	}
	oldest, err := store.Put("oldest.bin", "application/octet-stream", make([]byte, 450*1024))
	if err != nil {
		t.Fatalf("put oldest failed: %v", err)
	}
	newest, err := store.Put("newest.bin", "application/octet-stream", make([]byte, 450*1024))
	if err != nil {
		t.Fatalf("put newest failed: %v", err)
	}

	store.mu.Lock()
	pMeta := store.items[pinned.ID]
	pMeta.PinState = "pinned"
	pMeta.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
	pMeta.LastAccessAt = time.Now().UTC().Add(-2 * time.Hour)
	store.items[pinned.ID] = pMeta

	oMeta := store.items[oldest.ID]
	oMeta.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
	oMeta.LastAccessAt = time.Now().UTC().Add(-2 * time.Hour)
	store.items[oldest.ID] = oMeta

	nMeta := store.items[newest.ID]
	nMeta.CreatedAt = time.Now().UTC().Add(-10 * time.Minute)
	nMeta.LastAccessAt = time.Now().UTC().Add(-10 * time.Minute)
	store.items[newest.ID] = nMeta
	store.mu.Unlock()
	store.SetClassPolicies(0, 0, 1, 0) // file quota 1MB

	report, err := store.RunGC(time.Now().UTC(), 0, 3600, false)
	if err != nil {
		t.Fatalf("run gc failed: %v", err)
	}
	if report.DeletedByCause["expired"] != 1 {
		t.Fatalf("expected one expired eviction, got %+v", report)
	}
	if report.DeletedByCause["lru"] != 0 {
		t.Fatalf("did not expect lru eviction in this scenario, got %+v", report)
	}
	store.mu.RLock()
	_, pinnedExists := store.items[pinned.ID]
	_, oldestExists := store.items[oldest.ID]
	store.mu.RUnlock()
	if !pinnedExists {
		t.Fatal("pinned attachment must survive ttl gc")
	}
	if oldestExists {
		t.Fatal("oldest attachment must be expired")
	}

	// Add another file to exceed quota and force LRU eviction of non-pinned blobs.
	store.SetClassPolicies(0, 0, 0, 0)
	younger, err := store.Put("younger.bin", "application/octet-stream", make([]byte, 450*1024))
	if err != nil {
		t.Fatalf("put younger failed: %v", err)
	}
	store.mu.Lock()
	yMeta := store.items[younger.ID]
	yMeta.LastAccessAt = time.Now().UTC().Add(-1 * time.Minute)
	store.items[younger.ID] = yMeta
	store.mu.Unlock()
	store.SetClassPolicies(0, 0, 1, 0)

	report, err = store.RunGC(time.Now().UTC(), 0, 0, false)
	if err != nil {
		t.Fatalf("run gc failed: %v", err)
	}
	if report.DeletedByCause["lru"] < 1 {
		t.Fatalf("expected at least one lru eviction, got %+v", report)
	}
	store.mu.RLock()
	_, pinnedExists = store.items[pinned.ID]
	store.mu.RUnlock()
	if !pinnedExists {
		t.Fatal("pinned attachment must survive lru gc")
	}
}

func TestAttachmentStoreRunGCDryRunDoesNotDelete(t *testing.T) {
	store, err := NewAttachmentStore("")
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}
	first, err := store.Put("a.bin", "application/octet-stream", make([]byte, 700*1024))
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}
	second, err := store.Put("b.bin", "application/octet-stream", make([]byte, 700*1024))
	if err != nil {
		t.Fatalf("put second failed: %v", err)
	}
	store.SetClassPolicies(0, 0, 1, 0)

	report, err := store.RunGC(time.Now().UTC(), 0, 0, true)
	if err != nil {
		t.Fatalf("run dry gc failed: %v", err)
	}
	if !report.DryRun {
		t.Fatalf("expected dry run report")
	}
	if report.DeletedByCause["lru"] < 1 {
		t.Fatalf("expected dry run to report lru eviction candidates, got %+v", report)
	}
	store.mu.RLock()
	_, firstExists := store.items[first.ID]
	_, secondExists := store.items[second.ID]
	store.mu.RUnlock()
	if !firstExists {
		t.Fatal("dry run must not delete first item")
	}
	if !secondExists {
		t.Fatal("dry run must not delete second item")
	}
}

func TestAttachmentStoreHardCapBlocksWritesAtFullCapacity(t *testing.T) {
	store, err := NewAttachmentStore("")
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}
	store.SetClassPolicies(0, 0, 1, 0)
	store.SetHardCapPolicy(85, 100, 70)

	if _, err := store.Put("a.bin", "application/octet-stream", make([]byte, 1024*1024)); err != nil {
		t.Fatalf("put first file failed: %v", err)
	}
	if _, err := store.Put("b.bin", "application/octet-stream", []byte("x")); !errors.Is(err, ErrAttachmentHardCapReached) {
		t.Fatalf("expected ErrAttachmentHardCapReached, got: %v", err)
	}

	stats := store.HardCapStats()
	if stats["file_full_cap_hits"] < 1 {
		t.Fatalf("expected full cap hit metric, got: %+v", stats)
	}
}

func TestAttachmentStoreHighWatermarkTriggersAggressiveGC(t *testing.T) {
	store, err := NewAttachmentStore("")
	if err != nil {
		t.Fatalf("new attachment store failed: %v", err)
	}
	store.SetClassPolicies(0, 0, 10, 0)
	store.SetHardCapPolicy(85, 100, 60)

	if _, err := store.Put("f1.bin", "application/octet-stream", make([]byte, 4*1024*1024)); err != nil {
		t.Fatalf("put file #1 failed: %v", err)
	}
	if _, err := store.Put("f2.bin", "application/octet-stream", make([]byte, 4*1024*1024)); err != nil {
		t.Fatalf("put file #2 failed: %v", err)
	}
	if _, err := store.Put("f3.bin", "application/octet-stream", make([]byte, 1024*1024)); err != nil {
		t.Fatalf("put file #3 failed: %v", err)
	}

	report, err := store.RunGC(time.Now().UTC(), 0, 0, false)
	if err != nil {
		t.Fatalf("run gc failed: %v", err)
	}
	if report.DeletedByCause["lru"] < 1 {
		t.Fatalf("expected aggressive lru gc, got %+v", report)
	}
	usage := store.UsageByClass()["file"]
	if usage > int64(6*1024*1024) {
		t.Fatalf("expected usage reduced close to aggressive target, got %d", usage)
	}
}
