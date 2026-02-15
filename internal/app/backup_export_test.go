package app

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
	identity models.Identity
	contacts []models.Contact
}

func (f *fakeBackupIdentity) GetIdentity() models.Identity {
	return f.identity
}

func (f *fakeBackupIdentity) Contacts() []models.Contact {
	return append([]models.Contact(nil), f.contacts...)
}

type fakeBackupMessages struct {
	messages map[string]models.Message
	pending  map[string]storage.PendingMessage
}

func (f *fakeBackupMessages) Snapshot() (map[string]models.Message, map[string]storage.PendingMessage) {
	return f.messages, f.pending
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
		identity: models.Identity{ID: "id-1"},
		contacts: []models.Contact{{ID: "c-1", DisplayName: "Alice"}},
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
		Version  int             `json:"version"`
		Identity models.Identity `json:"identity"`
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
}
