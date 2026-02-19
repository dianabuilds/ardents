package daemonservice

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func newDaemonServiceForInboxAtomicityTest(t *testing.T) *Service {
	t.Helper()
	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func configureRequestInboxPersistFailure(t *testing.T, svc *Service) {
	t.Helper()
	badPath := filepath.Join(t.TempDir(), "request-inbox-as-dir")
	if err := os.MkdirAll(badPath, 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	svc.requestInboxState.Configure(badPath, "test-secret")
}

func TestPersistInboundRequestDoesNotMutateInboxOnPersistFailure(t *testing.T) {
	t.Parallel()
	svc := newDaemonServiceForInboxAtomicityTest(t)
	configureRequestInboxPersistFailure(t, svc)

	ok := svc.persistInboundRequest(models.Message{
		ID:        "req_atomic_1",
		ContactID: "aim1_contact_atomic",
		Content:   []byte("request"),
		Timestamp: time.Now().UTC(),
	})
	if ok {
		t.Fatal("expected persistInboundRequest to fail")
	}

	svc.requestRuntime.Mu.RLock()
	thread := svc.requestRuntime.Inbox["aim1_contact_atomic"]
	svc.requestRuntime.Mu.RUnlock()
	if len(thread) != 0 {
		t.Fatalf("inbox must stay unchanged on persist failure, got thread len=%d", len(thread))
	}
}

func TestTakeMessageRequestThreadDoesNotMutateInboxOnPersistFailure(t *testing.T) {
	t.Parallel()
	svc := newDaemonServiceForInboxAtomicityTest(t)
	configureRequestInboxPersistFailure(t, svc)

	svc.requestRuntime.Mu.Lock()
	svc.requestRuntime.Inbox["aim1_contact_atomic"] = []models.Message{{
		ID:        "req_atomic_2",
		ContactID: "aim1_contact_atomic",
		Content:   []byte("request"),
		Timestamp: time.Now().UTC(),
	}}
	svc.requestRuntime.Mu.Unlock()

	thread, found, err := svc.takeMessageRequestThread("aim1_contact_atomic")
	if err == nil {
		t.Fatal("expected takeMessageRequestThread to fail on persist")
	}
	if found {
		t.Fatal("found must be false when operation fails")
	}
	if thread != nil {
		t.Fatalf("thread must be nil on failed transaction, got len=%d", len(thread))
	}

	svc.requestRuntime.Mu.RLock()
	remaining := svc.requestRuntime.Inbox["aim1_contact_atomic"]
	svc.requestRuntime.Mu.RUnlock()
	if len(remaining) != 1 {
		t.Fatalf("inbox must remain intact on failure, got len=%d", len(remaining))
	}
}

func TestRemoveMessageRequestDoesNotMutateInboxOnPersistFailure(t *testing.T) {
	t.Parallel()
	svc := newDaemonServiceForInboxAtomicityTest(t)
	configureRequestInboxPersistFailure(t, svc)

	svc.requestRuntime.Mu.Lock()
	svc.requestRuntime.Inbox["aim1_contact_atomic"] = []models.Message{{
		ID:        "req_atomic_3",
		ContactID: "aim1_contact_atomic",
		Content:   []byte("request"),
		Timestamp: time.Now().UTC(),
	}}
	svc.requestRuntime.Mu.Unlock()

	removed, err := svc.removeMessageRequest("aim1_contact_atomic")
	if err == nil {
		t.Fatal("expected removeMessageRequest to fail on persist")
	}
	if removed {
		t.Fatal("removed must be false when operation fails")
	}

	svc.requestRuntime.Mu.RLock()
	remaining := svc.requestRuntime.Inbox["aim1_contact_atomic"]
	svc.requestRuntime.Mu.RUnlock()
	if len(remaining) != 1 {
		t.Fatalf("inbox must remain intact on failure, got len=%d", len(remaining))
	}
}
