package policy

import (
	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	"aim-chat/go-backend/pkg/models"
	"errors"
	"strings"
)

var ErrInvalidGroupWirePayload = errors.New("invalid group wire payload")

const GroupWireEventTypeMessage = "message"

func ValidateWirePayload(wire contracts.WirePayload) error {
	conversationType := strings.TrimSpace(wire.ConversationType)
	switch conversationType {
	case "", models.ConversationTypeDirect, models.ConversationTypeGroup:
	default:
		return ErrInvalidGroupWirePayload
	}

	hasGroupMetadata := strings.TrimSpace(wire.EventID) != "" ||
		strings.TrimSpace(wire.EventType) != "" ||
		wire.MembershipVersion > 0 ||
		wire.GroupKeyVersion > 0 ||
		strings.TrimSpace(wire.SenderDeviceID) != ""

	isGroup := conversationType == models.ConversationTypeGroup || hasGroupMetadata
	if !isGroup {
		return nil
	}
	if conversationType != models.ConversationTypeGroup {
		return ErrInvalidGroupWirePayload
	}
	if strings.TrimSpace(wire.ConversationID) == "" {
		return ErrInvalidGroupWirePayload
	}
	if strings.TrimSpace(wire.EventID) == "" {
		return ErrInvalidGroupWirePayload
	}
	if strings.TrimSpace(wire.SenderDeviceID) == "" {
		return ErrInvalidGroupWirePayload
	}
	if wire.MembershipVersion == 0 {
		return ErrInvalidGroupWirePayload
	}
	eventType := strings.TrimSpace(wire.EventType)
	if eventType == GroupWireEventTypeMessage {
		if wire.GroupKeyVersion == 0 {
			return ErrInvalidGroupWirePayload
		}
		return nil
	}
	if _, err := groupdomain.ParseGroupEventType(eventType); err != nil {
		return ErrInvalidGroupWirePayload
	}
	return nil
}
