package daemon

import (
	"path/filepath"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/storage"
)

type StorageBundle struct {
	MessageStore     *storage.MessageStore
	SessionStore     crypto.SessionStore
	AttachmentStore  *storage.AttachmentStore
	IdentityPath     string
	PrivacyPath      string
	BlocklistPath    string
	RequestInboxPath string
	GroupStatePath   string
	NodeBindingPath  string
}

func BuildStorageBundle(dataDir, secret string) (StorageBundle, error) {
	msgPath := filepath.Join(dataDir, "messages.json")
	sessionsPath := filepath.Join(dataDir, "sessions.json")
	attachmentsPath := filepath.Join(dataDir, "attachments")

	msgStore, err := storage.NewEncryptedPersistentMessageStore(msgPath, secret)
	if err != nil {
		return StorageBundle{}, err
	}
	attachmentStore, err := storage.NewAttachmentStoreWithSecret(attachmentsPath, secret)
	if err != nil {
		return StorageBundle{}, err
	}

	return StorageBundle{
		MessageStore:     msgStore,
		SessionStore:     crypto.NewEncryptedFileSessionStore(sessionsPath, secret),
		AttachmentStore:  attachmentStore,
		IdentityPath:     filepath.Join(dataDir, "identity.enc"),
		PrivacyPath:      filepath.Join(dataDir, "privacy.enc"),
		BlocklistPath:    filepath.Join(dataDir, "blocklist.enc"),
		RequestInboxPath: filepath.Join(dataDir, "requests.enc"),
		GroupStatePath:   filepath.Join(dataDir, "groups.enc"),
		NodeBindingPath:  filepath.Join(dataDir, "node_binding.enc"),
	}, nil
}
