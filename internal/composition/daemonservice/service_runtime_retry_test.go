package daemonservice

import (
	"testing"
	"time"

	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

func TestHandleRetryPublishErrorStopsAfterRetryCap(t *testing.T) {
	t.Parallel()

	store := storage.NewMessageStore()
	msg := models.Message{
		ID:        "msg-retry-cap",
		ContactID: "aim1_contact",
		Content:   []byte("payload"),
		Timestamp: time.Now().UTC(),
		Direction: "out",
		Status:    "pending",
	}
	if err := store.SaveMessage(msg); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if err := store.AddOrUpdatePending(msg, 8, time.Now().Add(time.Second), "network down"); err != nil {
		t.Fatalf("add pending: %v", err)
	}

	svc := &Service{
		messageStore: store,
		logger:       runtimeapp.DefaultLogger(),
		metrics:      runtimeapp.NewServiceMetricsState(),
		notifier:     runtimeapp.NewNotificationHub(32),
	}

	svc.handleRetryPublishError(storage.PendingMessage{
		Message:    msg,
		RetryCount: 8,
		NextRetry:  time.Now().Add(time.Second),
		LastError:  "network down",
	}, assertErr("network"))

	if count := store.PendingCount(); count != 0 {
		t.Fatalf("pending message must be removed after retry cap, got=%d", count)
	}
	updated, ok := store.GetMessage(msg.ID)
	if !ok {
		t.Fatalf("message must exist")
	}
	if updated.Status != "failed" {
		t.Fatalf("message status must be failed after retry cap, got=%q", updated.Status)
	}
}

func assertErr(text string) error {
	return &fakeErr{msg: text}
}

type fakeErr struct {
	msg string
}

func (e *fakeErr) Error() string { return e.msg }
