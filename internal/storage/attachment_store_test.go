package storage

import (
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
