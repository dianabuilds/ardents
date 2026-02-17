package messaging

import (
	"aim-chat/go-backend/pkg/models"
	"testing"
)

func TestValidateEditMessageInput(t *testing.T) {
	contactID, messageID, content, err := ValidateEditMessageInput(" c1 ", " m1 ", " hi ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contactID != "c1" || messageID != "m1" || content != "hi" {
		t.Fatalf("unexpected normalized values: %q %q %q", contactID, messageID, content)
	}
}

func TestEnsureEditableMessage(t *testing.T) {
	msg := models.Message{ID: "m1", ContactID: "c1", Direction: "out"}
	if err := EnsureEditableMessage(msg, true, "c1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := EnsureEditableMessage(msg, false, "c1"); err == nil {
		t.Fatal("expected message not found error")
	}
	if err := EnsureEditableMessage(msg, true, "c2"); err == nil {
		t.Fatal("expected contact mismatch error")
	}
	if err := EnsureEditableMessage(models.Message{ContactID: "c1", Direction: "in"}, true, "c1"); err == nil {
		t.Fatal("expected direction error")
	}
}

func TestBuildMessageStatus(t *testing.T) {
	status, err := BuildMessageStatus(models.Message{ID: "m1", Status: "sent"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.MessageID != "m1" || status.Status != "sent" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if _, err := BuildMessageStatus(models.Message{}, false); err == nil {
		t.Fatal("expected message not found error")
	}
}
