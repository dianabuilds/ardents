// noinspection GoNameStartsWithPackageName
package privacy

import (
	privacymodel "aim-chat/go-backend/internal/domains/privacy/model"
)

type MessagePrivacyMode = privacymodel.MessagePrivacyMode
type StorageProtectionMode = privacymodel.StorageProtectionMode
type ContentRetentionMode = privacymodel.ContentRetentionMode

const (
	MessagePrivacyContactsOnly        = privacymodel.MessagePrivacyContactsOnly
	MessagePrivacyRequests            = privacymodel.MessagePrivacyRequests
	MessagePrivacyEveryone            = privacymodel.MessagePrivacyEveryone
	DefaultMessagePrivacyMode         = privacymodel.DefaultMessagePrivacyMode
	StorageProtectionProtected        = privacymodel.StorageProtectionProtected
	DefaultStorageProtectionMode      = privacymodel.DefaultStorageProtectionMode
	RetentionEphemeral                = privacymodel.RetentionEphemeral
	RetentionZeroRetention            = privacymodel.RetentionZeroRetention
	DefaultContentRetentionMode       = privacymodel.DefaultContentRetentionMode
	DefaultEphemeralMessageTTLSeconds = privacymodel.DefaultEphemeralMessageTTLSeconds
	DefaultEphemeralFileTTLSeconds    = privacymodel.DefaultEphemeralFileTTLSeconds
)

var (
	ErrInvalidMessagePrivacyMode = privacymodel.ErrInvalidMessagePrivacyMode
	ErrInvalidIdentityID         = privacymodel.ErrInvalidIdentityID
)

// noinspection GoNameStartsWithPackageName
type PrivacySettings = privacymodel.PrivacySettings
type StoragePolicy = privacymodel.StoragePolicy
type Blocklist = privacymodel.Blocklist

func DefaultPrivacySettings() PrivacySettings {
	return privacymodel.DefaultPrivacySettings()
}

func NormalizePrivacySettings(in PrivacySettings) PrivacySettings {
	return privacymodel.NormalizePrivacySettings(in)
}

func ParseMessagePrivacyMode(raw string) (MessagePrivacyMode, error) {
	return privacymodel.ParseMessagePrivacyMode(raw)
}

func ParseStoragePolicy(storageProtection, retention string, messageTTLSeconds, fileTTLSeconds int) (StoragePolicy, error) {
	return privacymodel.ParseStoragePolicy(storageProtection, retention, messageTTLSeconds, fileTTLSeconds)
}

func NormalizeStoragePolicy(in StoragePolicy) StoragePolicy {
	return privacymodel.NormalizeStoragePolicy(in)
}

func StoragePolicyFromSettings(settings PrivacySettings) StoragePolicy {
	return privacymodel.StoragePolicyFromSettings(settings)
}

func NormalizeIdentityID(identityID string) (string, error) {
	return privacymodel.NormalizeIdentityID(identityID)
}

func NewBlocklist(ids []string) (Blocklist, error) {
	return privacymodel.NewBlocklist(ids)
}
