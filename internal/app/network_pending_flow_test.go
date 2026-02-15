package app

import (
	"context"
	"errors"
	"testing"

	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

func TestProcessPendingMessages(t *testing.T) {
	pending := []storage.PendingMessage{
		{Message: models.Message{ID: "m1", ContactID: "c1"}},
		{Message: models.Message{ID: "m2", ContactID: "c2"}},
	}
	errorCalls := 0
	sentCalls := 0

	ProcessPendingMessages(
		context.Background(),
		pending,
		func(msg models.Message) (WirePayload, error) { return NewPlainWire([]byte(msg.ID)), nil },
		func(ctx context.Context, messageID, recipient string, wire WirePayload) error {
			if messageID == "m1" {
				return errors.New("publish failed")
			}
			return nil
		},
		func(p storage.PendingMessage, err error) { errorCalls++ },
		func(messageID string) { sentCalls++ },
	)

	if errorCalls != 1 {
		t.Fatalf("errorCalls=%d want=1", errorCalls)
	}
	if sentCalls != 1 {
		t.Fatalf("sentCalls=%d want=1", sentCalls)
	}
}
