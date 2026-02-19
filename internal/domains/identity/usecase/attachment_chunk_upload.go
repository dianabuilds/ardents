package usecase

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	"aim-chat/go-backend/pkg/models"
)

type attachmentUploadSession struct {
	ID          string
	Name        string
	MimeType    string
	TotalSize   int64
	TotalChunks int
	ChunkSize   int
	FileSHA256  string
	Chunks      map[int][]byte
	UpdatedAt   time.Time
}

type AttachmentUploadInitResult struct {
	UploadID    string `json:"upload_id"`
	TotalChunks int    `json:"total_chunks"`
	ChunkSize   int    `json:"chunk_size"`
}

type AttachmentUploadChunkResult struct {
	UploadID      string `json:"upload_id"`
	ReceivedChunk int    `json:"received_chunk"`
	ReceivedCount int    `json:"received_count"`
	TotalChunks   int    `json:"total_chunks"`
}

type AttachmentUploadStatus struct {
	UploadID      string `json:"upload_id"`
	ReceivedCount int    `json:"received_count"`
	TotalChunks   int    `json:"total_chunks"`
	NextChunk     int    `json:"next_chunk"`
}

func (s *Service) InitAttachmentUpload(name, mimeType string, totalSize int64, totalChunks, chunkSize int, fileSHA256 string) (AttachmentUploadInitResult, error) {
	name, mimeType, totalSize, totalChunks, chunkSize, err := identitypolicy.ValidateAttachmentUploadInit(name, mimeType, totalSize, totalChunks, chunkSize)
	if err != nil {
		return AttachmentUploadInitResult{}, err
	}
	fileSHA256, err = identitypolicy.NormalizeOptionalSHA256Hex(fileSHA256, "file")
	if err != nil {
		return AttachmentUploadInitResult{}, err
	}
	uploadID, err := newUploadID()
	if err != nil {
		return AttachmentUploadInitResult{}, err
	}
	now := time.Now()
	s.purgeExpiredAttachmentUploads(now)
	s.uploadMu.Lock()
	s.uploads[uploadID] = attachmentUploadSession{
		ID:          uploadID,
		Name:        name,
		MimeType:    mimeType,
		TotalSize:   totalSize,
		TotalChunks: totalChunks,
		ChunkSize:   chunkSize,
		FileSHA256:  fileSHA256,
		Chunks:      make(map[int][]byte, totalChunks),
		UpdatedAt:   now,
	}
	s.uploadMu.Unlock()
	return AttachmentUploadInitResult{
		UploadID:    uploadID,
		TotalChunks: totalChunks,
		ChunkSize:   chunkSize,
	}, nil
}

func (s *Service) PutAttachmentChunk(uploadID string, chunkIndex int, dataBase64, chunkSHA256 string) (AttachmentUploadChunkResult, error) {
	chunkSHA256, err := identitypolicy.NormalizeOptionalSHA256Hex(chunkSHA256, "chunk")
	if err != nil {
		return AttachmentUploadChunkResult{}, err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(dataBase64))
	if err != nil {
		return AttachmentUploadChunkResult{}, errors.New("invalid attachment encoding")
	}

	s.purgeExpiredAttachmentUploads(time.Now())
	s.uploadMu.Lock()
	session, ok := s.uploads[strings.TrimSpace(uploadID)]
	if !ok {
		s.uploadMu.Unlock()
		return AttachmentUploadChunkResult{}, errors.New("upload session not found")
	}
	uploadID, err = identitypolicy.ValidateAttachmentChunkInput(uploadID, chunkIndex, data, session.ChunkSize, session.TotalSize, session.TotalChunks)
	if err != nil {
		s.uploadMu.Unlock()
		return AttachmentUploadChunkResult{}, err
	}
	if chunkSHA256 != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != chunkSHA256 {
			s.uploadMu.Unlock()
			return AttachmentUploadChunkResult{}, errors.New("chunk integrity check failed")
		}
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	session.Chunks[chunkIndex] = copied
	session.UpdatedAt = time.Now()
	s.uploads[uploadID] = session
	received := len(session.Chunks)
	totalChunks := session.TotalChunks
	s.uploadMu.Unlock()
	return AttachmentUploadChunkResult{
		UploadID:      uploadID,
		ReceivedChunk: chunkIndex,
		ReceivedCount: received,
		TotalChunks:   totalChunks,
	}, nil
}

func (s *Service) GetAttachmentUploadStatus(uploadID string) (AttachmentUploadStatus, error) {
	s.purgeExpiredAttachmentUploads(time.Now())
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	session, ok := s.uploads[strings.TrimSpace(uploadID)]
	if !ok {
		return AttachmentUploadStatus{}, errors.New("upload session not found")
	}
	next := 0
	for i := 0; i < session.TotalChunks; i++ {
		if _, exists := session.Chunks[i]; !exists {
			next = i
			break
		}
		next = i + 1
	}
	return AttachmentUploadStatus{
		UploadID:      session.ID,
		ReceivedCount: len(session.Chunks),
		TotalChunks:   session.TotalChunks,
		NextChunk:     next,
	}, nil
}

func (s *Service) CommitAttachmentUpload(uploadID string) (models.AttachmentMeta, error) {
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return models.AttachmentMeta{}, errors.New("upload id is required")
	}
	s.purgeExpiredAttachmentUploads(time.Now())
	s.uploadMu.Lock()
	session, ok := s.uploads[uploadID]
	if !ok {
		s.uploadMu.Unlock()
		return models.AttachmentMeta{}, errors.New("upload session not found")
	}
	data := make([]byte, 0, session.TotalSize)
	for i := 0; i < session.TotalChunks; i++ {
		chunk, exists := session.Chunks[i]
		if !exists {
			s.uploadMu.Unlock()
			return models.AttachmentMeta{}, errors.New("upload is incomplete")
		}
		data = append(data, chunk...)
	}
	if int64(len(data)) != session.TotalSize {
		s.uploadMu.Unlock()
		return models.AttachmentMeta{}, errors.New("invalid upload size")
	}
	if session.FileSHA256 != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != session.FileSHA256 {
			s.uploadMu.Unlock()
			return models.AttachmentMeta{}, errors.New("file integrity check failed")
		}
	}
	name, mimeType, normalized, err := identitypolicy.NormalizeChunkedAttachmentPayload(session.Name, session.MimeType, data)
	if err != nil {
		s.uploadMu.Unlock()
		return models.AttachmentMeta{}, err
	}
	delete(s.uploads, uploadID)
	s.uploadMu.Unlock()
	return s.attachmentStore.Put(name, mimeType, normalized)
}

func newUploadID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "upl_" + hex.EncodeToString(buf), nil
}
