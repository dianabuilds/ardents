package usecase

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"aim-chat/go-backend/pkg/models"
)

type chunkTestAttachmentStore struct {
	putFn func(name, mimeType string, data []byte) (models.AttachmentMeta, error)
}

func (s *chunkTestAttachmentStore) Put(name, mimeType string, data []byte) (models.AttachmentMeta, error) {
	if s.putFn != nil {
		return s.putFn(name, mimeType, data)
	}
	return models.AttachmentMeta{}, nil
}

func (s *chunkTestAttachmentStore) Get(_ string) (models.AttachmentMeta, []byte, error) {
	return models.AttachmentMeta{}, nil, nil
}

func TestAttachmentChunkUploadRoundtrip(t *testing.T) {
	payload := make([]byte, 20*1024)
	for i := range payload {
		payload[i] = byte('a' + (i % 26))
	}
	seen := []byte(nil)
	svc := &Service{
		attachmentStore: &chunkTestAttachmentStore{
			putFn: func(name, mimeType string, data []byte) (models.AttachmentMeta, error) {
				seen = append([]byte(nil), data...)
				return models.AttachmentMeta{ID: "att-1", Name: name, MimeType: mimeType, Size: int64(len(data))}, nil
			},
		},
		uploads: make(map[string]attachmentUploadSession),
	}

	initRes, err := svc.InitAttachmentUpload("doc.txt", "text/plain", int64(len(payload)), 2, 16*1024, "")
	if err != nil {
		t.Fatalf("init upload failed: %v", err)
	}
	chunk0 := base64.StdEncoding.EncodeToString(payload[:16*1024])
	chunk1 := base64.StdEncoding.EncodeToString(payload[16*1024:])
	if _, err := svc.PutAttachmentChunk(initRes.UploadID, 0, chunk0, ""); err != nil {
		t.Fatalf("put chunk 0 failed: %v", err)
	}
	if _, err := svc.PutAttachmentChunk(initRes.UploadID, 1, chunk1, ""); err != nil {
		t.Fatalf("put chunk 1 failed: %v", err)
	}
	meta, err := svc.CommitAttachmentUpload(initRes.UploadID)
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if meta.ID != "att-1" {
		t.Fatalf("unexpected meta id: %q", meta.ID)
	}
	if string(seen) != string(payload) {
		t.Fatalf("unexpected persisted payload: %q", string(seen))
	}
}

func TestAttachmentChunkUploadFailsOnDigestMismatch(t *testing.T) {
	payload := []byte("chunked-payload")
	svc := &Service{
		attachmentStore: &chunkTestAttachmentStore{},
		uploads:         make(map[string]attachmentUploadSession),
	}
	initRes, err := svc.InitAttachmentUpload("doc.txt", "text/plain", int64(len(payload)), 1, 16*1024, "")
	if err != nil {
		t.Fatalf("init upload failed: %v", err)
	}
	chunk := base64.StdEncoding.EncodeToString(payload)
	if _, err := svc.PutAttachmentChunk(initRes.UploadID, 0, chunk, strings.Repeat("a", 64)); err == nil {
		t.Fatal("expected chunk integrity error")
	}
}

func TestAttachmentChunkUploadStatusAndExpiry(t *testing.T) {
	payload := []byte("1234567890abcdef")
	svc := &Service{
		attachmentStore: &chunkTestAttachmentStore{},
		uploads:         make(map[string]attachmentUploadSession),
	}
	initRes, err := svc.InitAttachmentUpload("doc.txt", "text/plain", int64(len(payload)), 1, 16*1024, "")
	if err != nil {
		t.Fatalf("init upload failed: %v", err)
	}
	status, err := svc.GetAttachmentUploadStatus(initRes.UploadID)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.NextChunk != 0 {
		t.Fatalf("unexpected next chunk: %d", status.NextChunk)
	}

	svc.uploadMu.Lock()
	s := svc.uploads[initRes.UploadID]
	s.UpdatedAt = time.Now().Add(-20 * time.Minute)
	svc.uploads[initRes.UploadID] = s
	svc.uploadMu.Unlock()

	if _, err := svc.GetAttachmentUploadStatus(initRes.UploadID); err == nil {
		t.Fatal("expected expired session")
	}
}
