package app

import (
	"errors"
	"strings"
)

// MessagePrivacyMode defines which senders are allowed to start a conversation.
type MessagePrivacyMode string

const (
	MessagePrivacyContactsOnly MessagePrivacyMode = "contacts_only"
	MessagePrivacyRequests     MessagePrivacyMode = "requests"
	MessagePrivacyEveryone     MessagePrivacyMode = "everyone"
)

const DefaultMessagePrivacyMode = MessagePrivacyContactsOnly

var ErrInvalidMessagePrivacyMode = errors.New("invalid message privacy mode")

// PrivacySettings stores user-level inbound message privacy preferences.
type PrivacySettings struct {
	MessagePrivacyMode MessagePrivacyMode `json:"message_privacy_mode"`
}

func DefaultPrivacySettings() PrivacySettings {
	return PrivacySettings{
		MessagePrivacyMode: DefaultMessagePrivacyMode,
	}
}

func NormalizePrivacySettings(in PrivacySettings) PrivacySettings {
	if !in.MessagePrivacyMode.Valid() {
		in.MessagePrivacyMode = DefaultMessagePrivacyMode
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

func ParseMessagePrivacyMode(raw string) (MessagePrivacyMode, error) {
	mode := MessagePrivacyMode(strings.TrimSpace(raw))
	if !mode.Valid() {
		return "", ErrInvalidMessagePrivacyMode
	}
	return mode, nil
}
