package messaging

import (
	"context"
	"testing"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

var benchPendingErrors int
var benchPendingSent int

func BenchmarkProcessPendingMessagesHappyPath(b *testing.B) {
	pending := make([]storage.PendingMessage, 256)
	for i := range pending {
		pending[i] = storage.PendingMessage{Message: models.Message{
			ID:        "m",
			ContactID: "contact",
		}}
	}

	buildWire := func(_ models.Message) (contracts.WirePayload, error) {
		return NewPlainWire([]byte("payload")), nil
	}
	publish := func(_ context.Context, _, _ string, _ contracts.WirePayload) error {
		return nil
	}
	onError := func(storage.PendingMessage, error) {
		benchPendingErrors++
	}
	onSent := func(string) {
		benchPendingSent++
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ProcessPendingMessages(context.Background(), pending, buildWire, publish, onError, onSent)
	}
}
