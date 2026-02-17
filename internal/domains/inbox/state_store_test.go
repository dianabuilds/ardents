package inbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aim-chat/go-backend/internal/testutil/fsperm"
	"aim-chat/go-backend/pkg/models"
)

func TestRequestStoreBootstrapDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.enc")
	store := NewRequestStore()
	store.Configure(path, "test-secret")

	inbox, err := store.Bootstrap()
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	if len(inbox) != 0 {
		t.Fatalf("expected empty inbox, got %d entries", len(inbox))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected request inbox file to be created, err=%v", err)
	}
}

func TestRequestStorePersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.enc")
	store := NewRequestStore()
	store.Configure(path, "test-secret")

	now := time.Now().UTC()
	inbox := map[string][]models.Message{
		"aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF": {
			{
				ID:          "msg_1",
				ContactID:   "aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF",
				Content:     []byte("hello"),
				Timestamp:   now,
				Direction:   "in",
				Status:      "delivered",
				ContentType: "text",
			},
		},
	}
	if err := store.Persist(inbox); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	reload := NewRequestStore()
	reload.Configure(path, "test-secret")
	got, err := reload.Bootstrap()
	if err != nil {
		t.Fatalf("reload bootstrap failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one sender thread, got %d", len(got))
	}
	thread := got["aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF"]
	if len(thread) != 1 {
		t.Fatalf("expected one message in thread, got %d", len(thread))
	}
	if thread[0].ID != "msg_1" {
		t.Fatalf("unexpected message id: got=%q want=%q", thread[0].ID, "msg_1")
	}
	if string(thread[0].Content) != "hello" {
		t.Fatalf("unexpected message content: got=%q want=%q", string(thread[0].Content), "hello")
	}
}

func TestRequestStoreBootstrapIOError(t *testing.T) {
	dir := t.TempDir()
	store := NewRequestStore()
	store.Configure(dir, "test-secret")

	_, err := store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap io error")
	}
}

func TestRequestStorePersistCreatesPrivateDir(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "secure", "requests.enc")
	store := NewRequestStore()
	store.Configure(path, "test-secret")

	if err := store.Persist(map[string][]models.Message{}); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	fsperm.AssertPrivateDirPerm(t, filepath.Dir(path))
}
