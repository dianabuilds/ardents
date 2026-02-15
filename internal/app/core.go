package app

import "aim-chat/go-backend/pkg/models"

type CoreAPI interface {
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

	SendMessage(contactID, content string) (string, error)
	EditMessage(contactID, messageID, content string) (models.Message, error)
	GetMessages(contactID string, limit, offset int) ([]models.Message, error)
	GetMessageStatus(messageID string) (models.MessageStatus, error)
	InitSession(contactID string, peerPublicKey []byte) (models.SessionState, error)
	ListDevices() ([]models.Device, error)
	AddDevice(name string) (models.Device, error)
	RevokeDevice(deviceID string) (models.DeviceRevocation, error)

	GetNetworkStatus() models.NetworkStatus
	GetMetrics() models.MetricsSnapshot
}
