package contracts

import (
	"context"
	"log/slog"
	"time"

	"aim-chat/go-backend/internal/crypto"
	contractports "aim-chat/go-backend/internal/domains/contracts/ports"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

type SessionDomain interface {
	Snapshot() ([]crypto.SessionState, error)
	RestoreSnapshot(states []crypto.SessionState) error
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
	ListMessagesByConversation(conversationID, conversationType string, limit, offset int) []models.Message
	ListMessagesByConversationThread(conversationID, conversationType, threadID string, limit, offset int) []models.Message
	PendingCount() int
	DuePending(now time.Time) []storage.PendingMessage
}

type AttachmentRepository = contractports.AttachmentRepository

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

type ServiceOptions struct {
	SessionStore    crypto.SessionStore
	MessageStore    MessageRepository
	AttachmentStore AttachmentRepository
	Logger          *slog.Logger
}

type WirePayload struct {
	Kind              string                   `json:"kind"`
	Envelope          crypto.MessageEnvelope   `json:"envelope"`
	Plain             []byte                   `json:"plain"`
	Padding           string                   `json:"padding,omitempty"`
	ConversationID    string                   `json:"conversation_id,omitempty"`
	ConversationType  string                   `json:"conversation_type,omitempty"`
	ThreadID          string                   `json:"thread_id,omitempty"`
	EventID           string                   `json:"event_id,omitempty"`
	EventType         string                   `json:"event_type,omitempty"`
	MembershipVersion uint64                   `json:"membership_version,omitempty"`
	GroupKeyVersion   uint32                   `json:"group_key_version,omitempty"`
	SenderDeviceID    string                   `json:"sender_device_id,omitempty"`
	Card              *models.ContactCard      `json:"card,omitempty"`
	Receipt           *models.MessageReceipt   `json:"receipt,omitempty"`
	Device            *models.Device           `json:"device,omitempty"`
	DeviceSig         []byte                   `json:"device_sig,omitempty"`
	Revocation        *models.DeviceRevocation `json:"revocation,omitempty"`
}
