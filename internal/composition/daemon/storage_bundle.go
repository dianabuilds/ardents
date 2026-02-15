package daemon

import (
	"path/filepath"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/storage"
)

type StorageBundle struct {
	MessageStore    *storage.MessageStore
	SessionStore    crypto.SessionStore
	AttachmentStore *storage.AttachmentStore
	IdentityPath    string
}

func BuildStorageBundle(dataDir, secret string) (StorageBundle, error) {
	msgPath := filepath.Join(dataDir, "messages.json")
	sessionsPath := filepath.Join(dataDir, "sessions.json")
	attachmentsPath := filepath.Join(dataDir, "attachments")

	msgStore, err := storage.NewEncryptedPersistentMessageStore(msgPath, secret)
	if err != nil {
		return StorageBundle{}, err
	}
	attachmentStore, err := storage.NewAttachmentStore(attachmentsPath)
	if err != nil {
		return StorageBundle{}, err
	}

	return StorageBundle{
		MessageStore:    msgStore,
		SessionStore:    crypto.NewEncryptedFileSessionStore(sessionsPath, secret),
		AttachmentStore: attachmentStore,
		IdentityPath:    filepath.Join(dataDir, "identity.enc"),
	}, nil
}
