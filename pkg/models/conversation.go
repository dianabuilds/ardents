package models

import "strings"

const (
	ConversationTypeDirect = "direct"
	ConversationTypeGroup  = "group"
)

func NormalizeConversationType(raw string) string {
	switch strings.TrimSpace(raw) {
	case ConversationTypeGroup:
		return ConversationTypeGroup
	default:
		return ConversationTypeDirect
	}
}

func NormalizeMessageConversation(msg Message) Message {
	msg.ContactID = strings.TrimSpace(msg.ContactID)
	msg.ConversationID = strings.TrimSpace(msg.ConversationID)
	msg.ConversationType = NormalizeConversationType(msg.ConversationType)
	msg.ThreadID = strings.TrimSpace(msg.ThreadID)

	// Backward compatibility: direct messages default to contact-scoped conversation.
	if msg.ConversationType == ConversationTypeDirect && msg.ConversationID == "" {
		msg.ConversationID = msg.ContactID
	}
	return msg
}
