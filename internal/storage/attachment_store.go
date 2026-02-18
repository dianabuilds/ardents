package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/pkg/models"
)

var ErrAttachmentNotFound = errors.New("attachment not found")

type AttachmentStore struct {
	mu        sync.RWMutex
	dir       string
	indexPath string
	secret    string
	items     map[string]models.AttachmentMeta
	blobs     map[string][]byte
	persist   bool
}

func NewAttachmentStore(dir string) (*AttachmentStore, error) {
	return NewAttachmentStoreWithSecret(dir, "")
}

func NewAttachmentStoreWithSecret(dir, secret string) (*AttachmentStore, error) {
	s := &AttachmentStore{
		dir:     dir,
		secret:  strings.TrimSpace(secret),
		items:   make(map[string]models.AttachmentMeta),
		blobs:   make(map[string][]byte),
		persist: true,
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
		if !s.persist {
			s.items[id] = meta
			s.blobs[id] = append([]byte(nil), data...)
			return meta, nil
		}
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return models.AttachmentMeta{}, err
		}
		blob := append([]byte(nil), data...)
		if s.secret != "" {
			blob, err = securestore.Encrypt(s.secret, blob)
			if err != nil {
				return models.AttachmentMeta{}, err
			}
		}
		filePath := s.filePath(id)
		if err := os.WriteFile(filePath, blob, 0o600); err != nil {
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
	blob, hasBlob := s.blobs[id]
	s.mu.RUnlock()
	if !ok {
		return models.AttachmentMeta{}, nil, ErrAttachmentNotFound
	}
	if hasBlob {
		return meta, append([]byte(nil), blob...), nil
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
	if s.secret == "" {
		return meta, data, nil
	}
	plain, err := securestore.Decrypt(s.secret, data)
	if err != nil {
		// Backward compatibility for pre-protected plaintext attachments.
		if errors.Is(err, securestore.ErrLegacyData) {
			return meta, data, nil
		}
		return models.AttachmentMeta{}, nil, err
	}
	return meta, plain, nil
}

func (s *AttachmentStore) Wipe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]models.AttachmentMeta)
	s.blobs = make(map[string][]byte)
	if strings.TrimSpace(s.dir) == "" {
		return nil
	}
	if err := os.RemoveAll(s.dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *AttachmentStore) SetPersistenceEnabled(enabled bool) {
	s.mu.Lock()
	s.persist = enabled
	s.mu.Unlock()
}

func (s *AttachmentStore) PurgeOlderThan(cutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nextItems := make(map[string]models.AttachmentMeta, len(s.items))
	deletedIDs := make([]string, 0)
	for id, meta := range s.items {
		if !meta.CreatedAt.After(cutoff) {
			deletedIDs = append(deletedIDs, id)
			continue
		}
		nextItems[id] = meta
	}
	if len(deletedIDs) == 0 {
		return 0, nil
	}
	for _, id := range deletedIDs {
		delete(s.blobs, id)
		if s.persist && strings.TrimSpace(s.dir) != "" {
			if err := os.Remove(s.filePath(id)); err != nil && !os.IsNotExist(err) {
				return 0, err
			}
		}
	}
	if err := s.persistItemsLocked(nextItems); err != nil {
		return 0, err
	}
	s.items = nextItems
	return len(deletedIDs), nil
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
	indexWasLegacy := false
	if s.secret != "" {
		plain, derr := securestore.Decrypt(s.secret, data)
		if derr == nil {
			data = plain
		} else if errors.Is(derr, securestore.ErrLegacyData) {
			indexWasLegacy = true
		} else {
			return derr
		}
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
	if s.secret != "" {
		if err := s.migrateLegacyFiles(payload.Items); err != nil {
			return err
		}
		if indexWasLegacy {
			if err := s.persistItemsLocked(s.items); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *AttachmentStore) persistItemsLocked(items map[string]models.AttachmentMeta) error {
	if s.indexPath == "" || !s.persist {
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
	if s.secret != "" {
		data, err = securestore.Encrypt(s.secret, data)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(s.indexPath, data, 0o600)
}

func (s *AttachmentStore) migrateLegacyFiles(items map[string]models.AttachmentMeta) error {
	if s.secret == "" || len(items) == 0 {
		return nil
	}
	for id := range items {
		path := s.filePath(id)
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if _, derr := securestore.Decrypt(s.secret, raw); derr == nil {
			continue
		} else if !errors.Is(derr, securestore.ErrLegacyData) {
			return derr
		}
		enc, err := securestore.Encrypt(s.secret, raw)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, enc, 0o600); err != nil {
			return err
		}
	}
	return nil
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
