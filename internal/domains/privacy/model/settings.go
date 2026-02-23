package model

import (
	"errors"
	"fmt"
	"strings"
)

// MessagePrivacyMode defines which senders are allowed to start a conversation.
type MessagePrivacyMode string
type StorageProtectionMode string
type ContentRetentionMode string
type StoragePolicyScope string

const (
	MessagePrivacyContactsOnly MessagePrivacyMode = "contacts_only"
	MessagePrivacyRequests     MessagePrivacyMode = "requests"
	MessagePrivacyEveryone     MessagePrivacyMode = "everyone"

	StorageProtectionStandard  StorageProtectionMode = "standard"
	StorageProtectionProtected StorageProtectionMode = "protected"

	RetentionPersistent    ContentRetentionMode = "persistent"
	RetentionEphemeral     ContentRetentionMode = "ephemeral"
	RetentionZeroRetention ContentRetentionMode = "zero_retention"

	StoragePolicyScopeGlobal  StoragePolicyScope = "global"
	StoragePolicyScopeGroup   StoragePolicyScope = "group"
	StoragePolicyScopeChannel StoragePolicyScope = "channel"
	StoragePolicyScopeChat    StoragePolicyScope = "chat"
)

const DefaultMessagePrivacyMode = MessagePrivacyEveryone
const DefaultStorageProtectionMode = StorageProtectionStandard
const DefaultContentRetentionMode = RetentionPersistent
const DefaultEphemeralMessageTTLSeconds = 86400
const CurrentProfileSchemaVersion = 2

// DefaultEphemeralFileTTLSeconds Ephemeral mode keeps file blobs unless an explicit file TTL is provided.
const DefaultEphemeralFileTTLSeconds = 0

var ErrInvalidMessagePrivacyMode = errors.New("invalid message privacy mode")
var ErrInvalidStorageProtectionMode = errors.New("invalid storage protection mode")
var ErrInvalidContentRetentionMode = errors.New("invalid content retention mode")
var ErrInvalidTTLSeconds = errors.New("invalid ttl seconds")
var ErrInvalidStoragePolicyScope = errors.New("invalid storage policy scope")
var ErrInvalidStoragePolicyScopeID = errors.New("invalid storage policy scope id")
var ErrInfiniteTTLRequiresPinned = errors.New("infinite ttl requires pinned blob")

// PrivacySettings stores user-level inbound message privacy preferences.
type PrivacySettings struct {
	ProfileSchemaVersion  int                              `json:"profile_schema_version,omitempty"`
	MessagePrivacyMode    MessagePrivacyMode               `json:"message_privacy_mode"`
	StorageProtection     StorageProtectionMode            `json:"storage_protection_mode"`
	ContentRetentionMode  ContentRetentionMode             `json:"content_retention_mode"`
	MessageTTLSeconds     int                              `json:"message_ttl_seconds,omitempty"`
	ImageTTLSeconds       int                              `json:"image_ttl_seconds,omitempty"`
	FileTTLSeconds        int                              `json:"file_ttl_seconds,omitempty"`
	ImageQuotaMB          int                              `json:"image_quota_mb,omitempty"`
	FileQuotaMB           int                              `json:"file_quota_mb,omitempty"`
	ImageMaxItemSizeMB    int                              `json:"image_max_item_size_mb,omitempty"`
	FileMaxItemSizeMB     int                              `json:"file_max_item_size_mb,omitempty"`
	StorageScopeOverrides map[string]StoragePolicyOverride `json:"storage_scope_overrides,omitempty"`
	NodePolicies          *NodePolicies                    `json:"node_policies,omitempty"`
}

type StoragePolicy struct {
	StorageProtection    StorageProtectionMode `json:"storage_protection_mode"`
	ContentRetentionMode ContentRetentionMode  `json:"content_retention_mode"`
	MessageTTLSeconds    int                   `json:"message_ttl_seconds,omitempty"`
	ImageTTLSeconds      int                   `json:"image_ttl_seconds,omitempty"`
	FileTTLSeconds       int                   `json:"file_ttl_seconds,omitempty"`
	ImageQuotaMB         int                   `json:"image_quota_mb,omitempty"`
	FileQuotaMB          int                   `json:"file_quota_mb,omitempty"`
	ImageMaxItemSizeMB   int                   `json:"image_max_item_size_mb,omitempty"`
	FileMaxItemSizeMB    int                   `json:"file_max_item_size_mb,omitempty"`
}

type StoragePolicyOverride struct {
	StorageProtection      StorageProtectionMode `json:"storage_protection_mode"`
	ContentRetentionMode   ContentRetentionMode  `json:"content_retention_mode"`
	MessageTTLSeconds      int                   `json:"message_ttl_seconds,omitempty"`
	ImageTTLSeconds        int                   `json:"image_ttl_seconds,omitempty"`
	FileTTLSeconds         int                   `json:"file_ttl_seconds,omitempty"`
	ImageQuotaMB           int                   `json:"image_quota_mb,omitempty"`
	FileQuotaMB            int                   `json:"file_quota_mb,omitempty"`
	ImageMaxItemSizeMB     int                   `json:"image_max_item_size_mb,omitempty"`
	FileMaxItemSizeMB      int                   `json:"file_max_item_size_mb,omitempty"`
	InfiniteTTL            bool                  `json:"infinite_ttl,omitempty"`
	PinRequiredForInfinite bool                  `json:"pin_required_for_infinite,omitempty"`
}

type NodePolicies struct {
	ProfileSchemaVersion int                `json:"profile_schema_version,omitempty"`
	Personal             NodePersonalPolicy `json:"personal_policy"`
	Public               NodePublicPolicy   `json:"public_policy"`
}

type NodePersonalPolicy struct {
	StoreEnabled bool `json:"store_enabled"`
	TTLDays      int  `json:"ttl_days,omitempty"`
	QuotaMB      int  `json:"quota_mb,omitempty"`
	PinEnabled   bool `json:"pin_enabled,omitempty"`
}

type NodePublicPolicy struct {
	RelayEnabled     bool `json:"relay_enabled"`
	DiscoveryEnabled bool `json:"discovery_enabled"`
	ServingEnabled   bool `json:"serving_enabled"`
	StoreEnabled     bool `json:"store_enabled"`
	TTLDays          int  `json:"ttl_days,omitempty"`
	QuotaMB          int  `json:"quota_mb,omitempty"`
}

func DefaultNodePolicies() NodePolicies {
	return NodePolicies{
		ProfileSchemaVersion: CurrentProfileSchemaVersion,
		Personal: NodePersonalPolicy{
			StoreEnabled: true,
			TTLDays:      0,
			QuotaMB:      10 * 1024,
			PinEnabled:   true,
		},
		Public: NodePublicPolicy{
			RelayEnabled:     true,
			DiscoveryEnabled: true,
			ServingEnabled:   true,
			StoreEnabled:     false,
			TTLDays:          0,
			QuotaMB:          0,
		},
	}
}

func DefaultPrivacySettings() PrivacySettings {
	policies := DefaultNodePolicies()
	return PrivacySettings{
		ProfileSchemaVersion: CurrentProfileSchemaVersion,
		MessagePrivacyMode:   DefaultMessagePrivacyMode,
		StorageProtection:    DefaultStorageProtectionMode,
		ContentRetentionMode: DefaultContentRetentionMode,
		MessageTTLSeconds:    0,
		ImageTTLSeconds:      0,
		FileTTLSeconds:       0,
		ImageQuotaMB:         0,
		FileQuotaMB:          0,
		ImageMaxItemSizeMB:   0,
		FileMaxItemSizeMB:    0,
		NodePolicies:         &policies,
	}
}

func NormalizePrivacySettings(in PrivacySettings) PrivacySettings {
	in.ProfileSchemaVersion = normalizeProfileSchemaVersion(in.ProfileSchemaVersion)
	if !in.MessagePrivacyMode.Valid() {
		in.MessagePrivacyMode = DefaultMessagePrivacyMode
	}
	if !in.StorageProtection.Valid() {
		in.StorageProtection = DefaultStorageProtectionMode
	}
	if !in.ContentRetentionMode.Valid() {
		in.ContentRetentionMode = DefaultContentRetentionMode
	}
	in.MessageTTLSeconds = normalizeTTLSeconds(in.MessageTTLSeconds)
	in.ImageTTLSeconds = normalizeTTLSeconds(in.ImageTTLSeconds)
	in.FileTTLSeconds = normalizeTTLSeconds(in.FileTTLSeconds)
	in.ImageQuotaMB = normalizeLimitValue(in.ImageQuotaMB)
	in.FileQuotaMB = normalizeLimitValue(in.FileQuotaMB)
	in.ImageMaxItemSizeMB = normalizeLimitValue(in.ImageMaxItemSizeMB)
	in.FileMaxItemSizeMB = normalizeLimitValue(in.FileMaxItemSizeMB)
	in.StorageScopeOverrides = normalizeStorageScopeOverrides(in.StorageScopeOverrides)
	policies := normalizeNodePolicies(in.NodePolicies)
	in.NodePolicies = &policies
	if in.ContentRetentionMode != RetentionEphemeral {
		in.MessageTTLSeconds = 0
		in.ImageTTLSeconds = 0
		in.FileTTLSeconds = 0
	}
	if in.ContentRetentionMode == RetentionEphemeral {
		if in.MessageTTLSeconds == 0 {
			in.MessageTTLSeconds = DefaultEphemeralMessageTTLSeconds
		}
		if in.FileTTLSeconds == 0 {
			in.FileTTLSeconds = DefaultEphemeralFileTTLSeconds
		}
	}
	return in
}

func normalizeNodePolicies(in *NodePolicies) NodePolicies {
	base := DefaultNodePolicies()
	if in == nil {
		return base
	}
	base.ProfileSchemaVersion = normalizeProfileSchemaVersion(in.ProfileSchemaVersion)
	base.Personal.StoreEnabled = in.Personal.StoreEnabled
	base.Personal.PinEnabled = in.Personal.PinEnabled
	base.Personal.TTLDays = normalizeLimitValue(in.Personal.TTLDays)
	base.Personal.QuotaMB = normalizeLimitValue(in.Personal.QuotaMB)
	base.Public.RelayEnabled = in.Public.RelayEnabled
	base.Public.DiscoveryEnabled = in.Public.DiscoveryEnabled
	base.Public.ServingEnabled = in.Public.ServingEnabled
	base.Public.StoreEnabled = in.Public.StoreEnabled
	base.Public.TTLDays = normalizeLimitValue(in.Public.TTLDays)
	base.Public.QuotaMB = normalizeLimitValue(in.Public.QuotaMB)
	return base
}

func normalizeProfileSchemaVersion(v int) int {
	if v <= 0 {
		return CurrentProfileSchemaVersion
	}
	if v > CurrentProfileSchemaVersion {
		return CurrentProfileSchemaVersion
	}
	return v
}

func (m MessagePrivacyMode) Valid() bool {
	switch m {
	case MessagePrivacyContactsOnly, MessagePrivacyRequests, MessagePrivacyEveryone:
		return true
	default:
		return false
	}
}

func (m StorageProtectionMode) Valid() bool {
	switch m {
	case StorageProtectionStandard, StorageProtectionProtected:
		return true
	default:
		return false
	}
}

func (m ContentRetentionMode) Valid() bool {
	switch m {
	case RetentionPersistent, RetentionEphemeral, RetentionZeroRetention:
		return true
	default:
		return false
	}
}

func (s StoragePolicyScope) Valid() bool {
	switch s {
	case StoragePolicyScopeGlobal, StoragePolicyScopeGroup, StoragePolicyScopeChannel, StoragePolicyScopeChat:
		return true
	default:
		return false
	}
}

func ParseMessagePrivacyMode(raw string) (MessagePrivacyMode, error) {
	mode := MessagePrivacyMode(strings.TrimSpace(raw))
	if !mode.Valid() {
		return "", ErrInvalidMessagePrivacyMode
	}
	return mode, nil
}

func ParseStorageProtectionMode(raw string) (StorageProtectionMode, error) {
	mode := StorageProtectionMode(strings.TrimSpace(raw))
	if !mode.Valid() {
		return "", ErrInvalidStorageProtectionMode
	}
	return mode, nil
}

func ParseContentRetentionMode(raw string) (ContentRetentionMode, error) {
	mode := ContentRetentionMode(strings.TrimSpace(raw))
	if !mode.Valid() {
		return "", ErrInvalidContentRetentionMode
	}
	return mode, nil
}

func NormalizeStoragePolicy(in StoragePolicy) StoragePolicy {
	settings := NormalizePrivacySettings(PrivacySettings{
		MessagePrivacyMode:   DefaultMessagePrivacyMode,
		StorageProtection:    in.StorageProtection,
		ContentRetentionMode: in.ContentRetentionMode,
		MessageTTLSeconds:    in.MessageTTLSeconds,
		ImageTTLSeconds:      in.ImageTTLSeconds,
		FileTTLSeconds:       in.FileTTLSeconds,
		ImageQuotaMB:         in.ImageQuotaMB,
		FileQuotaMB:          in.FileQuotaMB,
		ImageMaxItemSizeMB:   in.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:    in.FileMaxItemSizeMB,
	})
	return StoragePolicy{
		StorageProtection:    settings.StorageProtection,
		ContentRetentionMode: settings.ContentRetentionMode,
		MessageTTLSeconds:    settings.MessageTTLSeconds,
		ImageTTLSeconds:      settings.ImageTTLSeconds,
		FileTTLSeconds:       settings.FileTTLSeconds,
		ImageQuotaMB:         settings.ImageQuotaMB,
		FileQuotaMB:          settings.FileQuotaMB,
		ImageMaxItemSizeMB:   settings.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:    settings.FileMaxItemSizeMB,
	}
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
	protectionMode, err := ParseStorageProtectionMode(storageProtection)
	if err != nil {
		return StoragePolicy{}, err
	}
	retentionMode, err := ParseContentRetentionMode(retention)
	if err != nil {
		return StoragePolicy{}, err
	}
	if messageTTLSeconds < 0 || imageTTLSeconds < 0 || fileTTLSeconds < 0 {
		return StoragePolicy{}, ErrInvalidTTLSeconds
	}
	if imageQuotaMB < 0 || fileQuotaMB < 0 || imageMaxItemSizeMB < 0 || fileMaxItemSizeMB < 0 {
		return StoragePolicy{}, ErrInvalidTTLSeconds
	}
	return NormalizeStoragePolicy(StoragePolicy{
		StorageProtection:    protectionMode,
		ContentRetentionMode: retentionMode,
		MessageTTLSeconds:    messageTTLSeconds,
		ImageTTLSeconds:      imageTTLSeconds,
		FileTTLSeconds:       fileTTLSeconds,
		ImageQuotaMB:         imageQuotaMB,
		FileQuotaMB:          fileQuotaMB,
		ImageMaxItemSizeMB:   imageMaxItemSizeMB,
		FileMaxItemSizeMB:    fileMaxItemSizeMB,
	}), nil
}

func StoragePolicyFromSettings(settings PrivacySettings) StoragePolicy {
	settings = NormalizePrivacySettings(settings)
	return StoragePolicy{
		StorageProtection:    settings.StorageProtection,
		ContentRetentionMode: settings.ContentRetentionMode,
		MessageTTLSeconds:    settings.MessageTTLSeconds,
		ImageTTLSeconds:      settings.ImageTTLSeconds,
		FileTTLSeconds:       settings.FileTTLSeconds,
		ImageQuotaMB:         settings.ImageQuotaMB,
		FileQuotaMB:          settings.FileQuotaMB,
		ImageMaxItemSizeMB:   settings.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:    settings.FileMaxItemSizeMB,
	}
}

func NormalizeStoragePolicyOverride(in StoragePolicyOverride) StoragePolicyOverride {
	base := NormalizeStoragePolicy(StoragePolicy{
		StorageProtection:    in.StorageProtection,
		ContentRetentionMode: in.ContentRetentionMode,
		MessageTTLSeconds:    in.MessageTTLSeconds,
		ImageTTLSeconds:      in.ImageTTLSeconds,
		FileTTLSeconds:       in.FileTTLSeconds,
		ImageQuotaMB:         in.ImageQuotaMB,
		FileQuotaMB:          in.FileQuotaMB,
		ImageMaxItemSizeMB:   in.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:    in.FileMaxItemSizeMB,
	})
	out := StoragePolicyOverride{
		StorageProtection:      base.StorageProtection,
		ContentRetentionMode:   base.ContentRetentionMode,
		MessageTTLSeconds:      base.MessageTTLSeconds,
		ImageTTLSeconds:        base.ImageTTLSeconds,
		FileTTLSeconds:         base.FileTTLSeconds,
		ImageQuotaMB:           base.ImageQuotaMB,
		FileQuotaMB:            base.FileQuotaMB,
		ImageMaxItemSizeMB:     base.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:      base.FileMaxItemSizeMB,
		InfiniteTTL:            in.InfiniteTTL,
		PinRequiredForInfinite: in.PinRequiredForInfinite,
	}
	if out.InfiniteTTL {
		out.ContentRetentionMode = RetentionPersistent
		out.MessageTTLSeconds = 0
		out.ImageTTLSeconds = 0
		out.FileTTLSeconds = 0
	}
	return out
}

func (in StoragePolicyOverride) Resolve(isPinned bool) (StoragePolicy, error) {
	normalized := NormalizeStoragePolicyOverride(in)
	if normalized.InfiniteTTL && normalized.PinRequiredForInfinite && !isPinned {
		return StoragePolicy{}, ErrInfiniteTTLRequiresPinned
	}
	return NormalizeStoragePolicy(StoragePolicy{
		StorageProtection:    normalized.StorageProtection,
		ContentRetentionMode: normalized.ContentRetentionMode,
		MessageTTLSeconds:    normalized.MessageTTLSeconds,
		ImageTTLSeconds:      normalized.ImageTTLSeconds,
		FileTTLSeconds:       normalized.FileTTLSeconds,
		ImageQuotaMB:         normalized.ImageQuotaMB,
		FileQuotaMB:          normalized.FileQuotaMB,
		ImageMaxItemSizeMB:   normalized.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:    normalized.FileMaxItemSizeMB,
	}), nil
}

func ScopeOverrideKey(scopeRaw, scopeIDRaw string) (string, error) {
	scope, scopeID, err := normalizeScope(scopeRaw, scopeIDRaw)
	if err != nil {
		return "", err
	}
	if scope == StoragePolicyScopeGlobal {
		return string(scope), nil
	}
	return fmt.Sprintf("%s:%s", scope, scopeID), nil
}

func ResolveStoragePolicyForScope(settings PrivacySettings, scopeRaw, scopeIDRaw string, isPinned bool) (StoragePolicy, error) {
	settings = NormalizePrivacySettings(settings)
	key, err := ScopeOverrideKey(scopeRaw, scopeIDRaw)
	if err != nil {
		return StoragePolicy{}, err
	}
	if override, ok := settings.StorageScopeOverrides[key]; ok {
		return override.Resolve(isPinned)
	}
	userDefault := StoragePolicyFromSettings(settings)
	if userDefault.StorageProtection.Valid() && userDefault.ContentRetentionMode.Valid() {
		return userDefault, nil
	}
	return NormalizeStoragePolicy(StoragePolicy{
		StorageProtection:    DefaultStorageProtectionMode,
		ContentRetentionMode: DefaultContentRetentionMode,
	}), nil
}

func normalizeTTLSeconds(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func normalizeLimitValue(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func normalizeStorageScopeOverrides(in map[string]StoragePolicyOverride) map[string]StoragePolicyOverride {
	if len(in) == 0 {
		return map[string]StoragePolicyOverride{}
	}
	out := make(map[string]StoragePolicyOverride, len(in))
	for key, override := range in {
		key = strings.TrimSpace(strings.ToLower(key))
		if key == "" {
			continue
		}
		out[key] = NormalizeStoragePolicyOverride(override)
	}
	return out
}

func normalizeScope(scopeRaw, scopeIDRaw string) (StoragePolicyScope, string, error) {
	scope := StoragePolicyScope(strings.ToLower(strings.TrimSpace(scopeRaw)))
	if !scope.Valid() {
		return "", "", ErrInvalidStoragePolicyScope
	}
	scopeID := strings.TrimSpace(scopeIDRaw)
	if scope == StoragePolicyScopeGlobal {
		return scope, "", nil
	}
	if scopeID == "" {
		return "", "", ErrInvalidStoragePolicyScopeID
	}
	return scope, scopeID, nil
}
