package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/pkg/models"
)

type PendingMessage struct {
	Message    models.Message `json:"message"`
	RetryCount int            `json:"retry_count"`
	NextRetry  time.Time      `json:"next_retry"`
	LastError  string         `json:"last_error"`
}

var ErrMessageIDConflict = errors.New("message id conflict")

type MessageStore struct {
	mu       sync.RWMutex
	messages map[string]models.Message
	pending  map[string]PendingMessage
	path     string
	secret   string
}

func NewMessageStore() *MessageStore {
	return &MessageStore{
		messages: make(map[string]models.Message),
		pending:  make(map[string]PendingMessage),
	}
}

func NewPersistentMessageStore(path string) (*MessageStore, error) {
	return NewEncryptedPersistentMessageStore(path, "")
}

func NewEncryptedPersistentMessageStore(path, passphrase string) (*MessageStore, error) {
	s := &MessageStore{
		messages: make(map[string]models.Message),
		pending:  make(map[string]PendingMessage),
		path:     path,
		secret:   passphrase,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *MessageStore) SaveMessage(msg models.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.messages[msg.ID]; ok {
		if messagesEqual(existing, msg) {
			return nil
		}
		return ErrMessageIDConflict
	}
	nextMessages := cloneMessagesMap(s.messages)
	nextMessages[msg.ID] = msg
	if err := s.persistSnapshotLocked(nextMessages, s.pending); err != nil {
		return err
	}
	s.messages = nextMessages
	return nil
}

func (s *MessageStore) UpdateMessageStatus(messageID, status string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg, ok := s.messages[messageID]
	if !ok {
		return false, nil
	}
	msg.Status = mergeMessageStatus(msg.Status, status)
	nextMessages := cloneMessagesMap(s.messages)
	nextMessages[messageID] = msg
	if err := s.persistSnapshotLocked(nextMessages, s.pending); err != nil {
		return false, err
	}
	s.messages = nextMessages
	return true, nil
}

func (s *MessageStore) UpdateMessageContent(messageID string, content []byte, contentType string) (models.Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg, ok := s.messages[messageID]
	if !ok {
		return models.Message{}, false, nil
	}
	msg.Content = append([]byte(nil), content...)
	msg.ContentType = contentType
	msg.Edited = true
	msg.Timestamp = time.Now().UTC()
	nextMessages := cloneMessagesMap(s.messages)
	nextMessages[messageID] = msg
	if err := s.persistSnapshotLocked(nextMessages, s.pending); err != nil {
		return models.Message{}, false, err
	}
	s.messages = nextMessages
	return msg, true, nil
}

func (s *MessageStore) DeleteMessage(contactID, messageID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg, ok := s.messages[messageID]
	if !ok || msg.ContactID != contactID {
		return false, nil
	}
	nextMessages := cloneMessagesMap(s.messages)
	delete(nextMessages, messageID)
	nextPending := clonePendingMap(s.pending)
	delete(nextPending, messageID)
	if err := s.persistSnapshotLocked(nextMessages, nextPending); err != nil {
		return false, err
	}
	s.messages = nextMessages
	s.pending = nextPending
	return true, nil
}

func (s *MessageStore) ClearMessages(contactID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nextMessages := make(map[string]models.Message, len(s.messages))
	deletedIDs := make(map[string]struct{})
	deleted := 0
	for id, msg := range s.messages {
		if msg.ContactID == contactID {
			deleted++
			deletedIDs[id] = struct{}{}
			continue
		}
		nextMessages[id] = msg
	}
	if deleted == 0 {
		return 0, nil
	}
	nextPending := make(map[string]PendingMessage, len(s.pending))
	for id, pending := range s.pending {
		if _, shouldDelete := deletedIDs[id]; shouldDelete {
			continue
		}
		if pending.Message.ContactID == contactID {
			continue
		}
		nextPending[id] = pending
	}
	if err := s.persistSnapshotLocked(nextMessages, nextPending); err != nil {
		return 0, err
	}
	s.messages = nextMessages
	s.pending = nextPending
	return deleted, nil
}

func (s *MessageStore) GetMessage(messageID string) (models.Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msg, ok := s.messages[messageID]
	if !ok {
		return models.Message{}, false
	}
	return msg, true
}

func (s *MessageStore) ListMessages(contactID string, limit, offset int) []models.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	filtered := make([]models.Message, 0)
	for _, msg := range s.messages {
		if msg.ContactID == contactID {
			filtered = append(filtered, msg)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	if offset < 0 {
		offset = 0
	}
	if offset >= len(filtered) {
		return []models.Message{}
	}
	filtered = filtered[offset:]
	if limit > 0 && limit < len(filtered) {
		return append([]models.Message(nil), filtered[:limit]...)
	}
	return append([]models.Message(nil), filtered...)
}

func (s *MessageStore) AddOrUpdatePending(message models.Message, retryCount int, nextRetry time.Time, lastErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nextPending := clonePendingMap(s.pending)
	nextPending[message.ID] = PendingMessage{
		Message:    message,
		RetryCount: retryCount,
		NextRetry:  nextRetry,
		LastError:  lastErr,
	}
	if err := s.persistSnapshotLocked(s.messages, nextPending); err != nil {
		return err
	}
	s.pending = nextPending
	return nil
}

func (s *MessageStore) RemovePending(messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nextPending := clonePendingMap(s.pending)
	delete(nextPending, messageID)
	if err := s.persistSnapshotLocked(s.messages, nextPending); err != nil {
		return err
	}
	s.pending = nextPending
	return nil
}

func (s *MessageStore) DuePending(now time.Time) []PendingMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PendingMessage, 0)
	for _, p := range s.pending {
		if !p.NextRetry.After(now) {
			out = append(out, p)
		}
	}
	return out
}

func (s *MessageStore) PendingCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pending)
}

func (s *MessageStore) Snapshot() (map[string]models.Message, map[string]PendingMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	messages := make(map[string]models.Message, len(s.messages))
	for k, v := range s.messages {
		messages[k] = v
	}
	pending := make(map[string]PendingMessage, len(s.pending))
	for k, v := range s.pending {
		pending[k] = v
	}
	return messages, pending
}

func (s *MessageStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var decoded []byte
	if s.secret != "" {
		decoded, err = securestore.Decrypt(s.secret, data)
		if err != nil {
			if errors.Is(err, securestore.ErrLegacyData) {
				decoded = data
			} else {
				return err
			}
		}
	} else {
		decoded = data
	}

	var snapshot struct {
		Messages map[string]models.Message `json:"messages"`
		Pending  map[string]PendingMessage `json:"pending"`
	}
	if err := json.Unmarshal(decoded, &snapshot); err != nil {
		return err
	}
	if snapshot.Messages != nil {
		s.messages = snapshot.Messages
	}
	if snapshot.Pending != nil {
		s.pending = snapshot.Pending
	}
	return nil
}

func (s *MessageStore) persistSnapshotLocked(messages map[string]models.Message, pending map[string]PendingMessage) error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	snapshot := struct {
		Messages map[string]models.Message `json:"messages"`
		Pending  map[string]PendingMessage `json:"pending"`
	}{
		Messages: messages,
		Pending:  pending,
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if s.secret != "" {
		data, err = securestore.Encrypt(s.secret, data)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(s.path, data, 0o600)
}

func cloneMessagesMap(in map[string]models.Message) map[string]models.Message {
	out := make(map[string]models.Message, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clonePendingMap(in map[string]PendingMessage) map[string]PendingMessage {
	out := make(map[string]PendingMessage, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeMessageStatus(current, candidate string) string {
	if statusOrder(candidate) >= statusOrder(current) {
		return candidate
	}
	return current
}

func statusOrder(status string) int {
	switch status {
	case "pending":
		return 1
	case "sent":
		return 2
	case "delivered":
		return 3
	case "read":
		return 4
	default:
		return 0
	}
}

func messagesEqual(a, b models.Message) bool {
	return a.ID == b.ID &&
		a.ContactID == b.ContactID &&
		bytes.Equal(a.Content, b.Content) &&
		a.Timestamp.Equal(b.Timestamp) &&
		a.Direction == b.Direction &&
		a.Status == b.Status &&
		a.ContentType == b.ContentType &&
		a.Edited == b.Edited
}
