package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"aim-chat/go-backend/pkg/models"
)

var ErrAttachmentNotFound = errors.New("attachment not found")

type AttachmentStore struct {
	mu        sync.RWMutex
	dir       string
	indexPath string
	items     map[string]models.AttachmentMeta
}

func NewAttachmentStore(dir string) (*AttachmentStore, error) {
	s := &AttachmentStore{
		dir:   dir,
		items: make(map[string]models.AttachmentMeta),
	}
	if dir != "" {
		s.indexPath = filepath.Join(dir, "index.json")
		if err := s.load(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *AttachmentStore) Put(name, mimeType string, data []byte) (models.AttachmentMeta, error) {
	if len(data) == 0 {
		return models.AttachmentMeta{}, errors.New("attachment data is empty")
	}
	id, err := newAttachmentID()
	if err != nil {
		return models.AttachmentMeta{}, err
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	meta := models.AttachmentMeta{
		ID:        id,
		Name:      name,
		MimeType:  mimeType,
		Size:      int64(len(data)),
		CreatedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir != "" {
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return models.AttachmentMeta{}, err
		}
		filePath := s.filePath(id)
		if err := os.WriteFile(filePath, data, 0o600); err != nil {
			return models.AttachmentMeta{}, err
		}
		nextItems := cloneAttachmentMetaMap(s.items)
		nextItems[id] = meta
		if err := s.persistItemsLocked(nextItems); err != nil {
			_ = os.Remove(filePath)
			return models.AttachmentMeta{}, err
		}
		s.items = nextItems
		return meta, nil
	}
	s.items[id] = meta
	return meta, nil
}

func (s *AttachmentStore) Get(id string) (models.AttachmentMeta, []byte, error) {
	s.mu.RLock()
	meta, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return models.AttachmentMeta{}, nil, ErrAttachmentNotFound
	}
	if s.dir == "" {
		return meta, nil, ErrAttachmentNotFound
	}
	data, err := os.ReadFile(s.filePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return models.AttachmentMeta{}, nil, ErrAttachmentNotFound
		}
		return models.AttachmentMeta{}, nil, err
	}
	return meta, data, nil
}

func (s *AttachmentStore) filePath(id string) string {
	return filepath.Join(s.dir, id+".bin")
}

func (s *AttachmentStore) load() error {
	if s.indexPath == "" {
		return nil
	}
	data, err := os.ReadFile(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var payload struct {
		Items map[string]models.AttachmentMeta `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.Items != nil {
		s.items = payload.Items
	}
	return nil
}

func (s *AttachmentStore) persistItemsLocked(items map[string]models.AttachmentMeta) error {
	if s.indexPath == "" {
		return nil
	}
	payload := struct {
		Items map[string]models.AttachmentMeta `json:"items"`
	}{
		Items: items,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath, data, 0o600)
}

func cloneAttachmentMetaMap(in map[string]models.AttachmentMeta) map[string]models.AttachmentMeta {
	out := make(map[string]models.AttachmentMeta, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func newAttachmentID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "att1_" + hex.EncodeToString(buf), nil
}
