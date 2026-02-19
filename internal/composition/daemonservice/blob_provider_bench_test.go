package daemonservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"aim-chat/go-backend/pkg/models"
)

func BenchmarkBlobProviderFetchFailover(b *testing.B) {
	registry := newBlobProviderRegistry()
	cfg := newMockConfig()

	svc, err := NewServiceForDaemonWithDataDir(cfg, b.TempDir())
	if err != nil {
		b.Fatalf("new service: %v", err)
	}
	if _, _, err := svc.CreateIdentity("bench-pass"); err != nil {
		b.Fatalf("create identity: %v", err)
	}
	useSharedBlobProviders(registry, svc)

	now := time.Now().UTC()
	_ = registry.announceBlob("bench_blob", "aim1bad", time.Minute, func(_ string, _ string) (models.AttachmentMeta, []byte, error) {
		return models.AttachmentMeta{}, nil, errors.New("boom")
	}, now)
	_ = registry.announceBlob("bench_blob", "aim1good", time.Minute, func(_ string, _ string) (models.AttachmentMeta, []byte, error) {
		return models.AttachmentMeta{ID: "bench_blob", Name: "ok.txt", MimeType: "text/plain"}, []byte("ok"), nil
	}, now)

	ctx := context.Background()
	requester := svc.localPeerID()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		meta, data, fetchErr := svc.fetchAttachmentFromProviders(ctx, "bench_blob", requester)
		if fetchErr != nil {
			b.Fatalf("fetchAttachmentFromProviders: %v", fetchErr)
		}
		if meta.ID == "" || len(data) == 0 {
			b.Fatalf("unexpected fetch result: meta=%+v data=%d", meta, len(data))
		}
	}
}
