package usecase

import (
	messagingpolicy "aim-chat/go-backend/internal/domains/messaging/policy"
	"aim-chat/go-backend/pkg/models"
	"time"
)

func ParseSendMessageInput(contactID, content string) (string, string, error) {
	return messagingpolicy.ValidateSendMessageInput(contactID, content)
}

func BuildOutboundDraft(messageID, contactID, content string, now time.Time) models.Message {
	return messagingpolicy.NewOutboundMessage(messageID, contactID, content, now)
}

func ParseEditMessageInput(contactID, messageID, content string) (string, string, string, error) {
	return messagingpolicy.ValidateEditMessageInput(contactID, messageID, content)
}

func ValidateEditableMessage(msg models.Message, found bool, contactID string) error {
	return messagingpolicy.EnsureEditableMessage(msg, found, contactID)
}

func ParseMessageListContactID(contactID string) (string, error) {
	return messagingpolicy.ValidateListMessagesContactID(contactID)
}

func ShouldAutoReadOnList(msg models.Message) bool {
	return messagingpolicy.ShouldAutoMarkRead(msg)
}

func ParseMessageStatusID(messageID string) (string, error) {
	return messagingpolicy.ValidateMessageStatusID(messageID)
}

func ComposeMessageStatus(msg models.Message, found bool) (models.MessageStatus, error) {
	return messagingpolicy.BuildMessageStatus(msg, found)
}

func NormalizeSessionContact(contactID string) string {
	return messagingpolicy.NormalizeSessionContactID(contactID)
}

func RequireVerifiedContact(verified bool) error {
	return messagingpolicy.EnsureVerifiedContact(verified)
}

func NormalizeDeviceIDForRevocation(deviceID string) string {
	return messagingpolicy.NormalizeDeviceID(deviceID)
}
