package daemonservice

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func TestWipeDataRemovesPersistedContentAndState(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	dataDir := t.TempDir()
	svc, err := NewServiceForDaemonWithDataDir(cfg, dataDir)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	msg := models.Message{
		ID:        "msg_wipe_1",
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
	meta, err := svc.attachmentStore.Put("a.txt", "text/plain", []byte("payload"))
	if err != nil {
		t.Fatalf("put attachment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "storage.key"), []byte("k"), 0o600); err != nil {
		t.Fatalf("write storage key: %v", err)
	}
	for _, name := range []string{"identity.enc", "privacy.enc", "blocklist.enc", "requests.enc", "groups.enc"} {
		if err := os.WriteFile(filepath.Join(dataDir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	ok, err := svc.WipeData("WRONG")
	if err == nil || ok {
		t.Fatalf("expected consent failure")
	}

	ok, err = svc.WipeData(DataWipeConsentToken)
	if err != nil {
		t.Fatalf("wipe failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected wiped=true")
	}

	messages, pending := svc.messageStore.Snapshot()
	if len(messages) != 0 || len(pending) != 0 {
		t.Fatalf("message store not wiped: messages=%d pending=%d", len(messages), len(pending))
	}
	sessions, err := svc.sessionManager.Snapshot()
	if err != nil {
		t.Fatalf("snapshot sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("session store not wiped: %d", len(sessions))
	}
	if _, _, err := svc.attachmentStore.Get(meta.ID); err == nil {
		t.Fatalf("expected attachment to be removed")
	}
	for _, name := range []string{"storage.key", "identity.enc", "privacy.enc", "blocklist.enc", "requests.enc", "groups.enc"} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed", name)
		}
	}
}
