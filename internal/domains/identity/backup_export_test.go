package identity

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

type fakeBackupIdentity struct {
	identity          models.Identity
	contacts          []models.Contact
	privateKey        []byte
	seedEnvelope      []byte
	restoreSeedCalled bool
}

func (f *fakeBackupIdentity) GetIdentity() models.Identity {
	return f.identity
}

func (f *fakeBackupIdentity) Contacts() []models.Contact {
	return append([]models.Contact(nil), f.contacts...)
}

func (f *fakeBackupIdentity) SnapshotIdentityKeys() ([]byte, []byte) {
	return append([]byte(nil), f.identity.SigningPublicKey...), append([]byte(nil), f.privateKey...)
}

func (f *fakeBackupIdentity) SnapshotSeedEnvelopeJSON() []byte {
	return append([]byte(nil), f.seedEnvelope...)
}

func (f *fakeBackupIdentity) RestoreIdentityPrivateKey(privateKey []byte) error {
	if len(privateKey) == 0 {
		return errors.New("missing private key")
	}
	f.privateKey = append([]byte(nil), privateKey...)
	f.identity = models.Identity{ID: "id-1"}
	return nil
}

func (f *fakeBackupIdentity) AddContactByIdentityID(contactID, displayName string) error {
	f.contacts = append(f.contacts, models.Contact{ID: contactID, DisplayName: displayName})
	return nil
}

func (f *fakeBackupIdentity) RestoreSeedEnvelopeJSON(raw []byte) error {
	f.restoreSeedCalled = true
	f.seedEnvelope = append([]byte(nil), raw...)
	return nil
}

type fakeBackupMessages struct {
	messages map[string]models.Message
	pending  map[string]storage.PendingMessage
}

func (f *fakeBackupMessages) Snapshot() (map[string]models.Message, map[string]storage.PendingMessage) {
	return f.messages, f.pending
}

func (f *fakeBackupMessages) SaveMessage(msg models.Message) error {
	if f.messages == nil {
		f.messages = make(map[string]models.Message)
	}
	f.messages[msg.ID] = msg
	return nil
}

func (f *fakeBackupMessages) AddOrUpdatePending(message models.Message, retryCount int, nextRetry time.Time, lastErr string) error {
	if f.pending == nil {
		f.pending = make(map[string]storage.PendingMessage)
	}
	f.pending[message.ID] = storage.PendingMessage{
		Message:    message,
		RetryCount: retryCount,
		NextRetry:  nextRetry,
		LastError:  lastErr,
	}
	return nil
}

func (f *fakeBackupMessages) Wipe() error {
	f.messages = make(map[string]models.Message)
	f.pending = make(map[string]storage.PendingMessage)
	return nil
}

type fakeBackupSessions struct {
	sessions []crypto.SessionState
	err      error
}

func (f *fakeBackupSessions) Snapshot() ([]crypto.SessionState, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]crypto.SessionState(nil), f.sessions...), nil
}

func (f *fakeBackupSessions) RestoreSnapshot(states []crypto.SessionState) error {
	f.sessions = append([]crypto.SessionState(nil), states...)
	return nil
}

func TestExportBackup_Validation(t *testing.T) {
	id := &fakeBackupIdentity{}
	msgs := &fakeBackupMessages{}
	sessions := &fakeBackupSessions{}

	if _, err := ExportBackup("", "pass", id, msgs, sessions); err == nil {
		t.Fatal("expected consent token error")
	}
	if _, err := ExportBackup("I_UNDERSTAND_BACKUP_RISK", "   ", id, msgs, sessions); err == nil {
		t.Fatal("expected passphrase error")
	}
}

func TestExportBackup_SessionSnapshotError(t *testing.T) {
	id := &fakeBackupIdentity{}
	msgs := &fakeBackupMessages{}
	sessions := &fakeBackupSessions{err: errors.New("snapshot failed")}

	if _, err := ExportBackup("I_UNDERSTAND_BACKUP_RISK", "pass", id, msgs, sessions); err == nil {
		t.Fatal("expected snapshot error")
	}
}

func TestExportBackup_Success(t *testing.T) {
	id := &fakeBackupIdentity{
		identity:     models.Identity{ID: "id-1"},
		contacts:     []models.Contact{{ID: "c-1", DisplayName: "Alice"}},
		privateKey:   []byte("private-key"),
		seedEnvelope: []byte(`{"version":1}`),
	}
	msgs := &fakeBackupMessages{
		messages: map[string]models.Message{
			"m-1": {ID: "m-1", ContactID: "c-1", Content: []byte("hi"), Timestamp: time.Now().UTC()},
		},
		pending: map[string]storage.PendingMessage{},
	}
	sessions := &fakeBackupSessions{sessions: []crypto.SessionState{}}

	result, err := ExportBackup("I_UNDERSTAND_BACKUP_RISK", "secret-pass", id, msgs, sessions)
	if err != nil {
		t.Fatalf("ExportBackup failed: %v", err)
	}
	if result.IdentityID != "id-1" {
		t.Fatalf("unexpected identity id: %s", result.IdentityID)
	}
	if result.MessageCount != 1 {
		t.Fatalf("unexpected message count: %d", result.MessageCount)
	}

	raw, err := base64.StdEncoding.DecodeString(result.Blob)
	if err != nil {
		t.Fatalf("blob is not base64: %v", err)
	}
	plain, err := securestore.Decrypt("secret-pass", raw)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	var payload struct {
		Version           int             `json:"version"`
		Identity          models.Identity `json:"identity"`
		SigningPrivateKey []byte          `json:"signing_private_key"`
		SeedEnvelope      []byte          `json:"seed_envelope"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatalf("invalid payload json: %v", err)
	}
	if payload.Version != 1 {
		t.Fatalf("unexpected payload version: %d", payload.Version)
	}
	if payload.Identity.ID != "id-1" {
		t.Fatalf("unexpected payload identity id: %s", payload.Identity.ID)
	}
	if len(payload.SigningPrivateKey) == 0 {
		t.Fatalf("expected private key snapshot in backup payload")
	}
	if string(payload.SeedEnvelope) != `{"version":1}` {
		t.Fatalf("unexpected seed envelope payload: %q", string(payload.SeedEnvelope))
	}
}

func TestRestoreBackup_Success(t *testing.T) {
	identity := &fakeBackupIdentity{
		identity:     models.Identity{ID: "id-1"},
		contacts:     []models.Contact{{ID: "c-1", DisplayName: "Alice"}},
		privateKey:   []byte("private-key"),
		seedEnvelope: []byte(`{"version":1}`),
	}
	messages := &fakeBackupMessages{
		messages: map[string]models.Message{
			"m-1": {ID: "m-1", ContactID: "c-1", Content: []byte("hi"), Timestamp: time.Now().UTC()},
		},
		pending: map[string]storage.PendingMessage{},
	}
	sessions := &fakeBackupSessions{
		sessions: []crypto.SessionState{{SessionID: "sess-1", ContactID: "c-1"}},
	}

	exported, err := ExportBackup("I_UNDERSTAND_BACKUP_RISK", "secret-pass", identity, messages, sessions)
	if err != nil {
		t.Fatalf("ExportBackup failed: %v", err)
	}

	restoredIdentity := &fakeBackupIdentity{}
	restoredMessages := &fakeBackupMessages{
		messages: map[string]models.Message{},
		pending:  map[string]storage.PendingMessage{},
	}
	restoredSessions := &fakeBackupSessions{}
	result, err := RestoreBackup("I_UNDERSTAND_BACKUP_RISK", "secret-pass", exported.Blob, restoredIdentity, restoredMessages, restoredSessions)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	if result.MessageCount != 1 || result.SessionCount != 1 {
		t.Fatalf("unexpected restore counts: %#v", result)
	}
	if restoredIdentity.identity.ID != "id-1" {
		t.Fatalf("expected restored identity to be initialized")
	}
	if !restoredIdentity.restoreSeedCalled {
		t.Fatalf("expected seed envelope restore to be called")
	}
	if len(restoredIdentity.contacts) != 1 || restoredIdentity.contacts[0].ID != "c-1" {
		t.Fatalf("unexpected restored contacts: %#v", restoredIdentity.contacts)
	}
	if len(restoredMessages.messages) != 1 {
		t.Fatalf("unexpected restored messages: %#v", restoredMessages.messages)
	}
	if len(restoredSessions.sessions) != 1 || restoredSessions.sessions[0].SessionID != "sess-1" {
		t.Fatalf("unexpected restored sessions: %#v", restoredSessions.sessions)
	}
}

func TestRestoreBackup_Validation(t *testing.T) {
	id := &fakeBackupIdentity{}
	msgs := &fakeBackupMessages{messages: map[string]models.Message{}, pending: map[string]storage.PendingMessage{}}
	sessions := &fakeBackupSessions{}

	if _, err := RestoreBackup("", "pass", "blob", id, msgs, sessions); err == nil {
		t.Fatal("expected consent token error")
	}
	if _, err := RestoreBackup("I_UNDERSTAND_BACKUP_RISK", "", "blob", id, msgs, sessions); err == nil {
		t.Fatal("expected passphrase error")
	}
	if _, err := RestoreBackup("I_UNDERSTAND_BACKUP_RISK", "pass", "", id, msgs, sessions); err == nil {
		t.Fatal("expected blob error")
	}
}
