package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

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
