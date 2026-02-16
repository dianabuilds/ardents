package app

import (
	"context"
	"log/slog"
	"time"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

type IdentityDomain interface {
	CreateIdentity(password string) (models.Identity, string, error)
	VerifyPassword(password string) error
	GetIdentity() models.Identity
	ExportSeed(password string) (string, error)
	ImportIdentity(mnemonic, password string) (models.Identity, error)
	ValidateMnemonic(mnemonic string) bool
	ChangePassword(oldPassword, newPassword string) error
	AddContact(card models.ContactCard) error
	VerifyContactCard(card models.ContactCard) (bool, error)
	AddContactByIdentityID(contactID, displayName string) error
	RemoveContact(contactID string) error
	Contacts() []models.Contact
	HasContact(contactID string) bool
	SelfContactCard(displayName string) (models.ContactCard, error)
	HasVerifiedContact(contactID string) bool
	ContactPublicKey(contactID string) ([]byte, bool)
	ApplyDeviceRevocation(contactID string, rev models.DeviceRevocation) error
	VerifyInboundDevice(contactID string, device models.Device, payload, sig []byte) error
	ListDevices() []models.Device
	AddDevice(name string) (models.Device, error)
	RevokeDevice(deviceID string) (models.DeviceRevocation, error)
	ActiveDeviceAuth(payload []byte) (models.Device, []byte, error)
	RestoreIdentityPrivateKey(privateKey []byte) error
	SnapshotIdentityKeys() (publicKey []byte, privateKey []byte)
}

type SessionDomain interface {
	Snapshot() ([]crypto.SessionState, error)
	GetSession(contactID string) (crypto.SessionState, bool, error)
	Encrypt(contactID string, plaintext []byte) (crypto.MessageEnvelope, error)
	Decrypt(contactID string, env crypto.MessageEnvelope) ([]byte, error)
	InitSession(localIdentityID, contactID string, peerPublicKey []byte) (crypto.SessionState, error)
}

type MessageRepository interface {
	SaveMessage(msg models.Message) error
	Snapshot() (map[string]models.Message, map[string]storage.PendingMessage)
	AddOrUpdatePending(message models.Message, retryCount int, nextRetry time.Time, lastErr string) error
	RemovePending(messageID string) error
	UpdateMessageStatus(messageID, status string) (bool, error)
	GetMessage(messageID string) (models.Message, bool)
	UpdateMessageContent(messageID string, content []byte, contentType string) (models.Message, bool, error)
	DeleteMessage(contactID, messageID string) (bool, error)
	ClearMessages(contactID string) (int, error)
	ListMessages(contactID string, limit, offset int) []models.Message
	PendingCount() int
	DuePending(now time.Time) []storage.PendingMessage
}

type AttachmentRepository interface {
	Put(name, mimeType string, data []byte) (models.AttachmentMeta, error)
	Get(id string) (models.AttachmentMeta, []byte, error)
}

type TransportNode interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status() waku.Status
	SetIdentity(identityID string)
	SubscribePrivate(handler func(waku.PrivateMessage)) error
	PublishPrivate(ctx context.Context, msg waku.PrivateMessage) error
	FetchPrivateSince(ctx context.Context, recipient string, since time.Time, limit int) ([]waku.PrivateMessage, error)
	ListenAddresses() []string
	NetworkMetrics() map[string]int
}

type NotificationBus interface {
	Subscribe(fromSeq int64) ([]NotificationEvent, <-chan NotificationEvent, func())
	Publish(method string, payload any) NotificationEvent
	BacklogSize() int
}

type ServiceOptions struct {
	SessionStore    crypto.SessionStore
	MessageStore    MessageRepository
	AttachmentStore AttachmentRepository
	Logger          *slog.Logger
}

const MaxAttachmentBytes = 5 << 20 // 5 MiB
const PublishTimeout = 5 * time.Second
