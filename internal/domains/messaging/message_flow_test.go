package messaging

import (
	"errors"
	"testing"
	"time"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

type fakeMessageSession struct {
	hasSession bool
	getErr     error
	encryptErr error
}

func (f *fakeMessageSession) GetSession(_ string) (crypto.SessionState, bool, error) {
	if f.getErr != nil {
		return crypto.SessionState{}, false, f.getErr
	}
	return crypto.SessionState{}, f.hasSession, nil
}

func (f *fakeMessageSession) Encrypt(_ string, _ []byte) (crypto.MessageEnvelope, error) {
	if f.encryptErr != nil {
		return crypto.MessageEnvelope{}, f.encryptErr
	}
	return crypto.MessageEnvelope{Ciphertext: []byte("enc")}, nil
}

func TestValidateSendMessageInput(t *testing.T) {
	contactID, content, err := ValidateSendMessageInput(" c1 ", " hi ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contactID != "c1" || content != "hi" {
		t.Fatalf("unexpected normalized values: %q %q", contactID, content)
	}
	if _, _, err := ValidateSendMessageInput("", "x"); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBuildWireForOutboundMessage(t *testing.T) {
	msg := NewOutboundMessage("m1", "c1", "hello", time.Now())
	session := &fakeMessageSession{hasSession: false}
	_, _, err := BuildWireForOutboundMessage(msg, session)
	if !errors.Is(err, ErrOutboundSessionRequired) {
		t.Fatalf("expected ErrOutboundSessionRequired, got %v", err)
	}

	session.hasSession = true
	wire, encrypted, err := BuildWireForOutboundMessage(msg, session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !encrypted || wire.Kind != "e2ee" {
		t.Fatalf("expected e2ee wire, got kind=%s encrypted=%v", wire.Kind, encrypted)
	}
}

func TestBuildWireForOutboundMessage_GetSessionError(t *testing.T) {
	msg := NewOutboundMessage("m1", "c1", "hello", time.Now())
	session := &fakeMessageSession{getErr: errors.New("session store unavailable")}
	_, _, err := BuildWireForOutboundMessage(msg, session)
	if err == nil {
		t.Fatal("expected error when session lookup fails")
	}
}

func TestShouldAutoMarkRead(t *testing.T) {
	if !ShouldAutoMarkRead(models.Message{Direction: "in", Status: "delivered"}) {
		t.Fatal("expected inbound delivered to be auto-marked read")
	}
	if ShouldAutoMarkRead(models.Message{Direction: "out", Status: "delivered"}) {
		t.Fatal("outbound message must not be auto-marked read")
	}
	if ShouldAutoMarkRead(models.Message{Direction: "in", Status: "read"}) {
		t.Fatal("already read message must not be auto-marked read")
	}
}

func TestAllocateOutboundMessage(t *testing.T) {
	now := time.Now()
	nextCalls := 0
	saveCalls := 0
	msg, err := AllocateOutboundMessage(
		"c1",
		"hello",
		func() time.Time { return now },
		func() (string, error) {
			nextCalls++
			return "m1", nil
		},
		func(_ models.Message) error {
			saveCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ID != "m1" || msg.ContactID != "c1" {
		t.Fatalf("unexpected message: %#v", msg)
	}
	if msg.ConversationType != models.ConversationTypeDirect || msg.ConversationID != "c1" {
		t.Fatalf("unexpected conversation fields: type=%q id=%q", msg.ConversationType, msg.ConversationID)
	}
	if nextCalls != 1 || saveCalls != 1 {
		t.Fatalf("unexpected call counts: next=%d save=%d", nextCalls, saveCalls)
	}
}

func TestAllocateOutboundMessage_RetryOnConflict(t *testing.T) {
	now := time.Now()
	idx := 0
	ids := []string{"m1", "m2"}
	saveCalls := 0
	msg, err := AllocateOutboundMessage(
		"c1",
		"hello",
		func() time.Time { return now },
		func() (string, error) {
			id := ids[idx]
			idx++
			return id, nil
		},
		func(_ models.Message) error {
			saveCalls++
			if saveCalls == 1 {
				return storage.ErrMessageIDConflict
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ID != "m2" {
		t.Fatalf("expected second id after conflict, got %s", msg.ID)
	}
}

func TestAllocateOutboundMessage_FailsAfterConflicts(t *testing.T) {
	_, err := AllocateOutboundMessage(
		"c1",
		"hello",
		time.Now,
		func() (string, error) { return "m", nil },
		func(_ models.Message) error { return storage.ErrMessageIDConflict },
	)
	if err == nil {
		t.Fatal("expected allocation error")
	}
}

func TestAllocateOutboundMessage_IDGeneratorError(t *testing.T) {
	want := errors.New("id gen failed")
	_, err := AllocateOutboundMessage(
		"c1",
		"hello",
		time.Now,
		func() (string, error) { return "", want },
		func(_ models.Message) error { return nil },
	)
	if !errors.Is(err, want) {
		t.Fatalf("expected id generator error, got %v", err)
	}
}
