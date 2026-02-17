package messaging

import (
	"aim-chat/go-backend/internal/domains/contracts"
	messagingpolicy "aim-chat/go-backend/internal/domains/messaging/policy"
	"aim-chat/go-backend/pkg/models"
	"time"
)

var ErrOutboundSessionRequired = messagingpolicy.ErrOutboundSessionRequired
var ErrInvalidGroupWirePayload = messagingpolicy.ErrInvalidGroupWirePayload

const GroupWireEventTypeMessage = messagingpolicy.GroupWireEventTypeMessage

func ValidateEditMessageInput(contactID, messageID, content string) (string, string, string, error) {
	return messagingpolicy.ValidateEditMessageInput(contactID, messageID, content)
}

func EnsureEditableMessage(msg models.Message, found bool, contactID string) error {
	return messagingpolicy.EnsureEditableMessage(msg, found, contactID)
}

func ValidateListMessagesContactID(contactID string) (string, error) {
	return messagingpolicy.ValidateListMessagesContactID(contactID)
}

func BuildMessageStatus(msg models.Message, found bool) (models.MessageStatus, error) {
	return messagingpolicy.BuildMessageStatus(msg, found)
}

func ValidateSendMessageInput(contactID, content string) (string, string, error) {
	return messagingpolicy.ValidateSendMessageInput(contactID, content)
}

func NewOutboundMessage(messageID, contactID, content string, now time.Time) models.Message {
	return messagingpolicy.NewOutboundMessage(messageID, contactID, content, now)
}

func ShouldAutoMarkRead(msg models.Message) bool {
	return messagingpolicy.ShouldAutoMarkRead(msg)
}

func NormalizeSessionContactID(contactID string) string {
	return messagingpolicy.NormalizeSessionContactID(contactID)
}

func EnsureVerifiedContact(verified bool) error {
	return messagingpolicy.EnsureVerifiedContact(verified)
}

func ValidateWirePayload(wire contracts.WirePayload) error {
	return messagingpolicy.ValidateWirePayload(wire)
}
