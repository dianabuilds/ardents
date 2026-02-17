package ports

import (
	"context"
	"time"

	groupdomain "aim-chat/go-backend/internal/domains/group"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/pkg/models"
)

// IdentityAPI is a transport-neutral identity/account contract.
type IdentityAPI interface {
	Logout() error

	GetIdentity() (models.Identity, error)
	SelfContactCard(displayName string) (models.ContactCard, error)
	CreateIdentity(password string) (models.Identity, string, error)
	ExportSeed(password string) (string, error)
	ExportBackup(consentToken, passphrase string) (string, error)
	ImportIdentity(mnemonic, password string) (models.Identity, error)
	ValidateMnemonic(mnemonic string) bool
	ChangePassword(oldPassword, newPassword string) error

	AddContactCard(card models.ContactCard) error
	VerifyContactCard(card models.ContactCard) (bool, error)
	PutAttachment(name, mimeType, dataBase64 string) (models.AttachmentMeta, error)
	GetAttachment(attachmentID string) (models.AttachmentMeta, []byte, error)

	AddContact(contactID, displayName string) error
	RemoveContact(contactID string) error
	GetContacts() ([]models.Contact, error)
	ListDevices() ([]models.Device, error)
	AddDevice(name string) (models.Device, error)
	RevokeDevice(deviceID string) (models.DeviceRevocation, error)
}

// MessagingAPI is a transport-neutral direct messaging/session contract.
type MessagingAPI interface {
	SendMessage(contactID, content string) (string, error)
	EditMessage(contactID, messageID, content string) (models.Message, error)
	DeleteMessage(contactID, messageID string) error
	ClearMessages(contactID string) (int, error)
	GetMessages(contactID string, limit, offset int) ([]models.Message, error)
	GetMessageStatus(messageID string) (models.MessageStatus, error)
	InitSession(contactID string, peerPublicKey []byte) (models.SessionState, error)
}

// GroupAPI is a transport-neutral group messaging contract.
type GroupAPI interface {
	CreateGroup(title string) (groupdomain.Group, error)
	GetGroup(groupID string) (groupdomain.Group, error)
	ListGroups() ([]groupdomain.Group, error)
	ListGroupMembers(groupID string) ([]groupdomain.GroupMember, error)
	LeaveGroup(groupID string) (bool, error)
	InviteToGroup(groupID, memberID string) (groupdomain.GroupMember, error)
	AcceptGroupInvite(groupID string) (bool, error)
	DeclineGroupInvite(groupID string) (bool, error)
	RemoveGroupMember(groupID, memberID string) (bool, error)
	PromoteGroupMember(groupID, memberID string) (groupdomain.GroupMember, error)
	DemoteGroupMember(groupID, memberID string) (groupdomain.GroupMember, error)
	SendGroupMessage(groupID, content string) (groupdomain.GroupMessageFanoutResult, error)
	ListGroupMessages(groupID string, limit, offset int) ([]models.Message, error)
	GetGroupMessageStatus(groupID, messageID string) (models.MessageStatus, error)
	DeleteGroupMessage(groupID, messageID string) error
}

// InboxAPI is a transport-neutral inbound request/review contract.
type InboxAPI interface {
	ListMessageRequests() ([]models.MessageRequest, error)
	GetMessageRequest(senderID string) (models.MessageRequestThread, error)
	AcceptMessageRequest(senderID string) (bool, error)
	DeclineMessageRequest(senderID string) (bool, error)
	BlockSender(senderID string) (models.BlockSenderResult, error)
}

// PrivacyAPI is a transport-neutral privacy settings contract.
type PrivacyAPI interface {
	GetPrivacySettings() (privacydomain.PrivacySettings, error)
	UpdatePrivacySettings(mode string) (privacydomain.PrivacySettings, error)
	GetBlocklist() ([]string, error)
	AddToBlocklist(identityID string) ([]string, error)
	RemoveFromBlocklist(identityID string) ([]string, error)
}

// NetworkAPI is a transport-neutral network/metrics read contract.
type NetworkAPI interface {
	GetNetworkStatus() models.NetworkStatus
	GetMetrics() models.MetricsSnapshot
}

// CoreAPI is a compatibility aggregate for transport-neutral contracts.
// Prefer using context-specific interfaces instead of this monolithic surface.
type CoreAPI interface {
	IdentityAPI
	MessagingAPI
	GroupAPI
	InboxAPI
	PrivacyAPI
	NetworkAPI
}

type DaemonService interface {
	IdentityAPI
	MessagingAPI
	GroupAPI
	InboxAPI
	PrivacyAPI
	NetworkAPI
	StartNetworking(ctx context.Context) error
	StopNetworking(ctx context.Context) error
	SubscribeNotifications(cursor int64) ([]NotificationEvent, <-chan NotificationEvent, func())
	ListenAddresses() []string
}

type NotificationEvent struct {
	Seq       int64
	Method    string
	Payload   any
	Timestamp time.Time
}

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

type AttachmentRepository interface {
	Put(name, mimeType string, data []byte) (models.AttachmentMeta, error)
	Get(id string) (models.AttachmentMeta, []byte, error)
}

type PrivacySettingsStateStore interface {
	Configure(path, secret string)
	Bootstrap() (privacydomain.PrivacySettings, error)
	Persist(settings privacydomain.PrivacySettings) error
}

type BlocklistStateStore interface {
	Configure(path, secret string)
	Bootstrap() (privacydomain.Blocklist, error)
	Persist(list privacydomain.Blocklist) error
}

type CategorizedError struct {
	Category string
	Err      error
}

func (e *CategorizedError) Error() string {
	return e.Err.Error()
}

func (e *CategorizedError) Unwrap() error {
	return e.Err
}

type DeviceRevocationDeliveryError struct {
	Attempted int
	Failed    int
	Failures  map[string]string
}

func (e *DeviceRevocationDeliveryError) Error() string {
	if e == nil {
		return "device revocation delivery failed"
	}
	if e.Attempted <= 0 {
		return "device revocation delivery failed: no recipients"
	}
	if e.Failed >= e.Attempted {
		return "device revocation delivery failed for all recipients"
	}
	return "device revocation delivery partially failed"
}

func (e *DeviceRevocationDeliveryError) IsFullFailure() bool {
	return e != nil && e.Attempted > 0 && e.Failed >= e.Attempted
}
