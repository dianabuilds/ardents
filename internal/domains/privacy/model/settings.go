package model

import (
	"errors"
	"strings"
)

// MessagePrivacyMode defines which senders are allowed to start a conversation.
type MessagePrivacyMode string
type StorageProtectionMode string
type ContentRetentionMode string

const (
	MessagePrivacyContactsOnly MessagePrivacyMode = "contacts_only"
	MessagePrivacyRequests     MessagePrivacyMode = "requests"
	MessagePrivacyEveryone     MessagePrivacyMode = "everyone"

	StorageProtectionStandard  StorageProtectionMode = "standard"
	StorageProtectionProtected StorageProtectionMode = "protected"

	RetentionPersistent    ContentRetentionMode = "persistent"
	RetentionEphemeral     ContentRetentionMode = "ephemeral"
	RetentionZeroRetention ContentRetentionMode = "zero_retention"
)

const DefaultMessagePrivacyMode = MessagePrivacyEveryone
const DefaultStorageProtectionMode = StorageProtectionStandard
const DefaultContentRetentionMode = RetentionPersistent
const DefaultEphemeralMessageTTLSeconds = 86400
const DefaultEphemeralFileTTLSeconds = 86400

var ErrInvalidMessagePrivacyMode = errors.New("invalid message privacy mode")
var ErrInvalidStorageProtectionMode = errors.New("invalid storage protection mode")
var ErrInvalidContentRetentionMode = errors.New("invalid content retention mode")
var ErrInvalidTTLSeconds = errors.New("invalid ttl seconds")

// PrivacySettings stores user-level inbound message privacy preferences.
type PrivacySettings struct {
	MessagePrivacyMode   MessagePrivacyMode    `json:"message_privacy_mode"`
	StorageProtection    StorageProtectionMode `json:"storage_protection_mode"`
	ContentRetentionMode ContentRetentionMode  `json:"content_retention_mode"`
	MessageTTLSeconds    int                   `json:"message_ttl_seconds,omitempty"`
	FileTTLSeconds       int                   `json:"file_ttl_seconds,omitempty"`
}

type StoragePolicy struct {
	StorageProtection    StorageProtectionMode `json:"storage_protection_mode"`
	ContentRetentionMode ContentRetentionMode  `json:"content_retention_mode"`
	MessageTTLSeconds    int                   `json:"message_ttl_seconds,omitempty"`
	FileTTLSeconds       int                   `json:"file_ttl_seconds,omitempty"`
}

func DefaultPrivacySettings() PrivacySettings {
	return PrivacySettings{
		MessagePrivacyMode:   DefaultMessagePrivacyMode,
		StorageProtection:    DefaultStorageProtectionMode,
		ContentRetentionMode: DefaultContentRetentionMode,
		MessageTTLSeconds:    0,
		FileTTLSeconds:       0,
	}
}

func NormalizePrivacySettings(in PrivacySettings) PrivacySettings {
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
	in.FileTTLSeconds = normalizeTTLSeconds(in.FileTTLSeconds)
	if in.ContentRetentionMode != RetentionEphemeral {
		in.MessageTTLSeconds = 0
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
		FileTTLSeconds:       in.FileTTLSeconds,
	})
	return StoragePolicy{
		StorageProtection:    settings.StorageProtection,
		ContentRetentionMode: settings.ContentRetentionMode,
		MessageTTLSeconds:    settings.MessageTTLSeconds,
		FileTTLSeconds:       settings.FileTTLSeconds,
	}
}

func ParseStoragePolicy(storageProtection, retention string, messageTTLSeconds, fileTTLSeconds int) (StoragePolicy, error) {
	protectionMode, err := ParseStorageProtectionMode(storageProtection)
	if err != nil {
		return StoragePolicy{}, err
	}
	retentionMode, err := ParseContentRetentionMode(retention)
	if err != nil {
		return StoragePolicy{}, err
	}
	if messageTTLSeconds < 0 || fileTTLSeconds < 0 {
		return StoragePolicy{}, ErrInvalidTTLSeconds
	}
	return NormalizeStoragePolicy(StoragePolicy{
		StorageProtection:    protectionMode,
		ContentRetentionMode: retentionMode,
		MessageTTLSeconds:    messageTTLSeconds,
		FileTTLSeconds:       fileTTLSeconds,
	}), nil
}

func StoragePolicyFromSettings(settings PrivacySettings) StoragePolicy {
	settings = NormalizePrivacySettings(settings)
	return StoragePolicy{
		StorageProtection:    settings.StorageProtection,
		ContentRetentionMode: settings.ContentRetentionMode,
		MessageTTLSeconds:    settings.MessageTTLSeconds,
		FileTTLSeconds:       settings.FileTTLSeconds,
	}
}

func normalizeTTLSeconds(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
