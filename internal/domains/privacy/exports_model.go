// noinspection GoNameStartsWithPackageName
package privacy

import (
	privacymodel "aim-chat/go-backend/internal/domains/privacy/model"
)

type MessagePrivacyMode = privacymodel.MessagePrivacyMode
type StorageProtectionMode = privacymodel.StorageProtectionMode
type ContentRetentionMode = privacymodel.ContentRetentionMode
type StoragePolicyScope = privacymodel.StoragePolicyScope

const (
	MessagePrivacyContactsOnly        = privacymodel.MessagePrivacyContactsOnly
	MessagePrivacyRequests            = privacymodel.MessagePrivacyRequests
	MessagePrivacyEveryone            = privacymodel.MessagePrivacyEveryone
	DefaultMessagePrivacyMode         = privacymodel.DefaultMessagePrivacyMode
	StorageProtectionStandard         = privacymodel.StorageProtectionStandard
	StorageProtectionProtected        = privacymodel.StorageProtectionProtected
	DefaultStorageProtectionMode      = privacymodel.DefaultStorageProtectionMode
	RetentionPersistent               = privacymodel.RetentionPersistent
	RetentionEphemeral                = privacymodel.RetentionEphemeral
	RetentionZeroRetention            = privacymodel.RetentionZeroRetention
	DefaultContentRetentionMode       = privacymodel.DefaultContentRetentionMode
	DefaultEphemeralMessageTTLSeconds = privacymodel.DefaultEphemeralMessageTTLSeconds
	DefaultEphemeralFileTTLSeconds    = privacymodel.DefaultEphemeralFileTTLSeconds
	CurrentProfileSchemaVersion       = privacymodel.CurrentProfileSchemaVersion
)

var (
	ErrInvalidMessagePrivacyMode = privacymodel.ErrInvalidMessagePrivacyMode
	ErrInvalidIdentityID         = privacymodel.ErrInvalidIdentityID
	ErrInfiniteTTLRequiresPinned = privacymodel.ErrInfiniteTTLRequiresPinned
)

// noinspection GoNameStartsWithPackageName
type PrivacySettings = privacymodel.PrivacySettings
type StoragePolicy = privacymodel.StoragePolicy
type StoragePolicyOverride = privacymodel.StoragePolicyOverride
type NodePolicies = privacymodel.NodePolicies
type NodePersonalPolicy = privacymodel.NodePersonalPolicy
type NodePublicPolicy = privacymodel.NodePublicPolicy
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

func ParseStoragePolicy(
	storageProtection,
	retention string,
	messageTTLSeconds,
	imageTTLSeconds,
	fileTTLSeconds,
	imageQuotaMB,
	fileQuotaMB,
	imageMaxItemSizeMB,
	fileMaxItemSizeMB int,
) (StoragePolicy, error) {
	return privacymodel.ParseStoragePolicy(
		storageProtection,
		retention,
		messageTTLSeconds,
		imageTTLSeconds,
		fileTTLSeconds,
		imageQuotaMB,
		fileQuotaMB,
		imageMaxItemSizeMB,
		fileMaxItemSizeMB,
	)
}

func NormalizeStoragePolicy(in StoragePolicy) StoragePolicy {
	return privacymodel.NormalizeStoragePolicy(in)
}

func DefaultNodePolicies() NodePolicies {
	return privacymodel.DefaultNodePolicies()
}

func StoragePolicyFromSettings(settings PrivacySettings) StoragePolicy {
	return privacymodel.StoragePolicyFromSettings(settings)
}

func ScopeOverrideKey(scopeRaw, scopeIDRaw string) (string, error) {
	return privacymodel.ScopeOverrideKey(scopeRaw, scopeIDRaw)
}

func ResolveStoragePolicyForScope(settings PrivacySettings, scopeRaw, scopeIDRaw string, isPinned bool) (StoragePolicy, error) {
	return privacymodel.ResolveStoragePolicyForScope(settings, scopeRaw, scopeIDRaw, isPinned)
}

func NormalizeIdentityID(identityID string) (string, error) {
	return privacymodel.NormalizeIdentityID(identityID)
}

func NewBlocklist(ids []string) (Blocklist, error) {
	return privacymodel.NewBlocklist(ids)
}
