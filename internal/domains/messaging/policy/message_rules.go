package policy

import (
	"aim-chat/go-backend/pkg/models"
	"errors"
	"strings"
	"time"
)

var (
	ErrOutboundSessionRequired = errors.New("outbound session is required")
	errInvalidEditMessageInput = errors.New("contact id, message id and content are required")
	errMessageNotFound         = errors.New("message not found")
	errMessageWrongContact     = errors.New("message does not belong to contact")
	errMessageNotOutbound      = errors.New("only outbound messages can be edited")
	errContactIDRequired       = errors.New("contact id is required")
	errMessageIDRequired       = errors.New("message id is required")
	errInvalidSendMessageInput = errors.New("contact id and content are required")
	errContactNotVerified      = errors.New("contact is not verified")
)

func ValidateEditMessageInput(contactID, messageID, content string) (string, string, string, error) {
	contactID = strings.TrimSpace(contactID)
	messageID = strings.TrimSpace(messageID)
	content = strings.TrimSpace(content)
	if contactID == "" || messageID == "" || content == "" {
		return "", "", "", errInvalidEditMessageInput
	}
	return contactID, messageID, content, nil
}

func EnsureEditableMessage(msg models.Message, found bool, contactID string) error {
	if !found {
		return errMessageNotFound
	}
	if msg.ContactID != contactID {
		return errMessageWrongContact
	}
	if msg.Direction != "out" {
		return errMessageNotOutbound
	}
	return nil
}

func ValidateListMessagesContactID(contactID string) (string, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return "", errContactIDRequired
	}
	return contactID, nil
}

func ValidateMessageStatusID(messageID string) (string, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return "", errMessageIDRequired
	}
	return messageID, nil
}

func BuildMessageStatus(msg models.Message, found bool) (models.MessageStatus, error) {
	if !found {
		return models.MessageStatus{}, errMessageNotFound
	}
	return models.MessageStatus{MessageID: msg.ID, Status: msg.Status}, nil
}

func ValidateSendMessageInput(contactID, content string) (string, string, error) {
	contactID = strings.TrimSpace(contactID)
	content = strings.TrimSpace(content)
	if contactID == "" || content == "" {
		return "", "", errInvalidSendMessageInput
	}
	return contactID, content, nil
}

func NewOutboundMessage(messageID, contactID, content string, now time.Time) models.Message {
	return models.Message{
		ID:               messageID,
		ContactID:        contactID,
		ConversationID:   contactID,
		ConversationType: models.ConversationTypeDirect,
		Content:          []byte(content),
		Timestamp:        now.UTC(),
		Direction:        "out",
		Status:           "pending",
		ContentType:      "text",
	}
}

func ShouldAutoMarkRead(msg models.Message) bool {
	return msg.Direction == "in" && msg.Status != "read"
}

func NormalizeDeviceID(deviceID string) string {
	return strings.TrimSpace(deviceID)
}

func NormalizeSessionContactID(contactID string) string {
	return strings.TrimSpace(contactID)
}

func EnsureVerifiedContact(verified bool) error {
	if !verified {
		return errContactNotVerified
	}
	return nil
}
