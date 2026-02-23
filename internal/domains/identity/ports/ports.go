package ports

import (
	"time"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

type IdentityStateStore interface {
	Persist(identityManager contracts.IdentityDomain) error
}

type BackupIdentityReader interface {
	GetIdentity() models.Identity
	Contacts() []models.Contact
}

type BackupIdentitySnapshotter interface {
	SnapshotIdentityKeys() (publicKey []byte, privateKey []byte)
	SnapshotSeedEnvelopeJSON() []byte
}

type BackupIdentityRestorer interface {
	GetIdentity() models.Identity
	RestoreIdentityPrivateKey(privateKey []byte) error
	AddContactByIdentityID(contactID, displayName string) error
}

type BackupMessageSnapshotter interface {
	Snapshot() (map[string]models.Message, map[string]storage.PendingMessage)
}

type BackupMessageRestorer interface {
	SaveMessage(msg models.Message) error
	AddOrUpdatePending(message models.Message, retryCount int, nextRetry time.Time, lastErr string) error
}

type BackupSessionSnapshotter interface {
	Snapshot() ([]crypto.SessionState, error)
}

type BackupSessionRestorer interface {
	RestoreSnapshot(states []crypto.SessionState) error
}

type AccountIdentityAccess interface {
	GetIdentity() models.Identity
	VerifyPassword(seedPassword string) error
}

type CreateAccountIdentity interface {
	CreateIdentity(seedPassword string) (models.Identity, string, error)
}

type CreateIdentityAccess interface {
	CreateIdentity(seedPassword string) (models.Identity, string, error)
}

type ImportIdentityAccess interface {
	ImportIdentity(mnemonic, seedPassword string) (models.Identity, error)
}
