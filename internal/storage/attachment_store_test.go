package storage

import (
	"os"
	"path/filepath"
	"testing"

	"aim-chat/go-backend/pkg/models"
)

func TestAttachmentStorePutRollbackOnPersistError(t *testing.T) {
	dir := t.TempDir()
	indexAsDir := filepath.Join(dir, "index-as-dir")
	if err := os.MkdirAll(indexAsDir, 0o755); err != nil {
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
