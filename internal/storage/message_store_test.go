package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/pkg/models"
)

func TestMessageStatusMonotonicTransitions(t *testing.T) {
	s := NewMessageStore()
	msg := models.Message{
		ID:        "m1",
		ContactID: "c1",
		Status:    "pending",
		Timestamp: time.Now().UTC(),
	}
	if err := s.SaveMessage(msg); err != nil {
		t.Fatalf("save message failed: %v", err)
	}

	if _, err := s.UpdateMessageStatus("m1", "sent"); err != nil {
		t.Fatalf("set sent failed: %v", err)
	}
	if _, err := s.UpdateMessageStatus("m1", "read"); err != nil {
		t.Fatalf("set read failed: %v", err)
	}
	if _, err := s.UpdateMessageStatus("m1", "delivered"); err != nil {
		t.Fatalf("set delivered failed: %v", err)
	}

	got, ok := s.GetMessage("m1")
	if !ok {
		t.Fatal("message not found")
	}
	if got.Status != "read" {
		t.Fatalf("expected final status read, got %s", got.Status)
	}
}

func TestMessageStatusAllowsPendingToFailedButKeepsDelivered(t *testing.T) {
	s := NewMessageStore()
	now := time.Now().UTC()

	if err := s.SaveMessage(models.Message{
		ID:        "m-fail-1",
		ContactID: "c1",
		Status:    "pending",
		Timestamp: now,
	}); err != nil {
		t.Fatalf("save pending message failed: %v", err)
	}
	if _, err := s.UpdateMessageStatus("m-fail-1", "failed"); err != nil {
		t.Fatalf("set failed status failed: %v", err)
	}
	got, ok := s.GetMessage("m-fail-1")
	if !ok {
		t.Fatal("message m-fail-1 not found")
	}
	if got.Status != "failed" {
		t.Fatalf("expected status failed, got %s", got.Status)
	}

	if err := s.SaveMessage(models.Message{
		ID:        "m-fail-2",
		ContactID: "c1",
		Status:    "delivered",
		Timestamp: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("save delivered message failed: %v", err)
	}
	if _, err := s.UpdateMessageStatus("m-fail-2", "failed"); err != nil {
		t.Fatalf("attempt downgrade to failed should not error: %v", err)
	}
	got2, ok := s.GetMessage("m-fail-2")
	if !ok {
		t.Fatal("message m-fail-2 not found")
	}
	if got2.Status != "delivered" {
		t.Fatalf("delivered status must not downgrade to failed, got %s", got2.Status)
	}
}

func TestEncryptedPersistentMessageStoreTamperFailsAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages.enc")
	store, err := NewEncryptedPersistentMessageStore(path, "pass")
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	if err := store.SaveMessage(models.Message{
		ID:        "m2",
		ContactID: "c2",
		Status:    "pending",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save message failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	data[len(data)-3] ^= 0xFF
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write tampered file failed: %v", err)
	}

	_, err = NewEncryptedPersistentMessageStore(path, "pass")
	if !errors.Is(err, securestore.ErrAuthFailed) && !errors.Is(err, securestore.ErrInvalid) {
		t.Fatalf("expected ErrAuthFailed or ErrInvalid, got %v", err)
	}
}

func TestMessageStoreRejectsMessageIDConflict(t *testing.T) {
	s := NewMessageStore()
	base := models.Message{
		ID:          "dup-1",
		ContactID:   "c1",
		Content:     []byte("first"),
		Timestamp:   time.Now().UTC(),
		Direction:   "in",
		Status:      "delivered",
		ContentType: "text",
	}
	if err := s.SaveMessage(base); err != nil {
		t.Fatalf("save base message failed: %v", err)
	}

	conflict := base
	conflict.Content = []byte("second")
	if err := s.SaveMessage(conflict); !errors.Is(err, ErrMessageIDConflict) {
		t.Fatalf("expected ErrMessageIDConflict, got %v", err)
	}
}

func TestMessageStoreSaveMessageRollbackOnPersistError(t *testing.T) {
	store := &MessageStore{
		messages: make(map[string]models.Message),
		pending:  make(map[string]PendingMessage),
		path:     t.TempDir(), // directory path forces os.WriteFile error
		persist:  true,
	}
	msg := models.Message{
		ID:        "m-rollback",
		ContactID: "c1",
		Status:    "pending",
		Timestamp: time.Now().UTC(),
	}
	if err := store.SaveMessage(msg); err == nil {
		t.Fatal("expected save error")
	}
	if _, ok := store.GetMessage(msg.ID); ok {
		t.Fatal("message must not stay in memory after persist failure")
	}
}

func TestMessageStoreUpdateStatusRollbackOnPersistError(t *testing.T) {
	store := &MessageStore{
		messages: map[string]models.Message{
			"m1": {
				ID:        "m1",
				ContactID: "c1",
				Status:    "pending",
				Timestamp: time.Now().UTC(),
			},
		},
		pending: make(map[string]PendingMessage),
		path:    t.TempDir(), // directory path forces os.WriteFile error
		persist: true,
	}
	ok, err := store.UpdateMessageStatus("m1", "sent")
	if err == nil {
		t.Fatal("expected update error")
	}
	if ok {
		t.Fatal("expected false on failed persist")
	}
	got, exists := store.GetMessage("m1")
	if !exists {
		t.Fatal("message should still exist")
	}
	if got.Status != "pending" {
		t.Fatalf("status changed in memory on persist failure: %s", got.Status)
	}
}

func TestMessageStoreDeleteAndClearMessages(t *testing.T) {
	s := NewMessageStore()
	now := time.Now().UTC()
	items := []models.Message{
		{ID: "m1", ContactID: "c1", Timestamp: now},
		{ID: "m2", ContactID: "c1", Timestamp: now.Add(time.Second)},
		{ID: "m3", ContactID: "c2", Timestamp: now.Add(2 * time.Second)},
	}
	for _, msg := range items {
		if err := s.SaveMessage(msg); err != nil {
			t.Fatalf("save message failed: %v", err)
		}
	}

	deleted, err := s.DeleteMessage("c1", "m1")
	if err != nil {
		t.Fatalf("delete message failed: %v", err)
	}
	if !deleted {
		t.Fatal("expected message to be deleted")
	}
	if _, ok := s.GetMessage("m1"); ok {
		t.Fatal("message m1 should be deleted")
	}

	cleared, err := s.ClearMessages("c1")
	if err != nil {
		t.Fatalf("clear messages failed: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("expected cleared=1, got %d", cleared)
	}
	msgsC1 := s.ListMessages("c1", 10, 0)
	if len(msgsC1) != 0 {
		t.Fatalf("expected c1 history empty, got %d", len(msgsC1))
	}
	msgsC2 := s.ListMessages("c2", 10, 0)
	if len(msgsC2) != 1 {
		t.Fatalf("expected c2 history preserved, got %d", len(msgsC2))
	}
}

func TestEncryptedPersistentMessageStoreCreatesPrivateDir(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "secure", "messages.enc")
	store, err := NewEncryptedPersistentMessageStore(path, "pass")
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	if err := store.SaveMessage(models.Message{
		ID:        "m-private-dir",
		ContactID: "c-private",
		Status:    "pending",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save message failed: %v", err)
	}

	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir failed: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("expected dir perm 0700, got %04o", info.Mode().Perm())
	}
}

func TestMessageStoreNormalizesDirectConversationFields(t *testing.T) {
	s := NewMessageStore()
	msg := models.Message{
		ID:        "m-direct",
		ContactID: "c1",
		Content:   []byte("hi"),
		Timestamp: time.Now().UTC(),
		Direction: "out",
		Status:    "pending",
	}
	if err := s.SaveMessage(msg); err != nil {
		t.Fatalf("save message failed: %v", err)
	}
	got, ok := s.GetMessage("m-direct")
	if !ok {
		t.Fatal("message not found")
	}
	if got.ConversationType != models.ConversationTypeDirect {
		t.Fatalf("expected direct conversation type, got %q", got.ConversationType)
	}
	if got.ConversationID != "c1" {
		t.Fatalf("expected conversation id to match contact id, got %q", got.ConversationID)
	}
}

func TestMessageStoreListMessagesByConversationSupportsGroupAndDirect(t *testing.T) {
	s := NewMessageStore()
	now := time.Now().UTC()

	fixture := []models.Message{
		{
			ID:               "m1",
			ContactID:        "c1",
			ConversationID:   "c1",
			ConversationType: models.ConversationTypeDirect,
			Timestamp:        now,
		},
		{
			ID:               "m2",
			ContactID:        "c2",
			ConversationID:   "group-1",
			ConversationType: models.ConversationTypeGroup,
			Timestamp:        now.Add(time.Second),
		},
		{
			ID:               "m3",
			ContactID:        "c3",
			ConversationID:   "group-1",
			ConversationType: models.ConversationTypeGroup,
			Timestamp:        now.Add(2 * time.Second),
		},
	}
	for _, msg := range fixture {
		if err := s.SaveMessage(msg); err != nil {
			t.Fatalf("save message %s failed: %v", msg.ID, err)
		}
	}

	direct := s.ListMessagesByConversation("c1", models.ConversationTypeDirect, 50, 0)
	if len(direct) != 1 || direct[0].ID != "m1" {
		t.Fatalf("unexpected direct conversation messages: %+v", direct)
	}

	group := s.ListMessagesByConversation("group-1", models.ConversationTypeGroup, 50, 0)
	if len(group) != 2 || group[0].ID != "m2" || group[1].ID != "m3" {
		t.Fatalf("unexpected group conversation messages: %+v", group)
	}

	legacyDirect := s.ListMessages("c1", 50, 0)
	if len(legacyDirect) != 1 || legacyDirect[0].ID != "m1" {
		t.Fatalf("legacy direct list regression: %+v", legacyDirect)
	}
}

func TestMessageStoreListMessagesByConversationThread(t *testing.T) {
	s := NewMessageStore()
	now := time.Now().UTC()

	fixture := []models.Message{
		{
			ID:               "m1",
			ContactID:        "c1",
			ConversationID:   "group-1",
			ConversationType: models.ConversationTypeGroup,
			ThreadID:         "t1",
			Timestamp:        now,
		},
		{
			ID:               "m2",
			ContactID:        "c2",
			ConversationID:   "group-1",
			ConversationType: models.ConversationTypeGroup,
			ThreadID:         "t2",
			Timestamp:        now.Add(time.Second),
		},
		{
			ID:               "m3",
			ContactID:        "c3",
			ConversationID:   "group-1",
			ConversationType: models.ConversationTypeGroup,
			ThreadID:         "t1",
			Timestamp:        now.Add(2 * time.Second),
		},
	}
	for _, msg := range fixture {
		if err := s.SaveMessage(msg); err != nil {
			t.Fatalf("save message %s failed: %v", msg.ID, err)
		}
	}

	thread := s.ListMessagesByConversationThread("group-1", models.ConversationTypeGroup, "t1", 50, 0)
	if len(thread) != 2 || thread[0].ID != "m1" || thread[1].ID != "m3" {
		t.Fatalf("unexpected thread messages: %+v", thread)
	}
}

func TestMessageStorePurgeOlderThan(t *testing.T) {
	s := NewMessageStore()
	now := time.Now().UTC()

	oldMsg := models.Message{
		ID:        "old",
		ContactID: "c1",
		Content:   []byte("old"),
		Timestamp: now.Add(-2 * time.Hour),
	}
	newMsg := models.Message{
		ID:        "new",
		ContactID: "c1",
		Content:   []byte("new"),
		Timestamp: now,
	}
	if err := s.SaveMessage(oldMsg); err != nil {
		t.Fatalf("save old message failed: %v", err)
	}
	if err := s.SaveMessage(newMsg); err != nil {
		t.Fatalf("save new message failed: %v", err)
	}
	if err := s.AddOrUpdatePending(oldMsg, 1, now, "retry"); err != nil {
		t.Fatalf("add pending failed: %v", err)
	}

	deleted, err := s.PurgeOlderThan(now.Add(-30 * time.Minute))
	if err != nil {
		t.Fatalf("purge failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one deleted message, got %d", deleted)
	}
	if _, ok := s.GetMessage("old"); ok {
		t.Fatal("old message must be deleted")
	}
	if _, ok := s.GetMessage("new"); !ok {
		t.Fatal("new message must stay")
	}
	if s.PendingCount() != 0 {
		t.Fatalf("pending entries for deleted messages must be removed, got %d", s.PendingCount())
	}
}
