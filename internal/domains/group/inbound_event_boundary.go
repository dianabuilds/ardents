package group

import (
	"encoding/json"
	"strings"
	"time"
)

type inboundGroupEventPayload struct {
	MemberID    string `json:"member_id"`
	Role        string `json:"role"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Avatar      string `json:"avatar"`
	KeyVersion  uint32 `json:"key_version"`
	OccurredAt  string `json:"occurred_at"`
}

type InboundGroupEventWire struct {
	EventID           string
	ConversationID    string
	MembershipVersion uint64
	EventType         string
	Plain             []byte
	SenderID          string
	RecipientID       string
}

func DecodeInboundGroupEvent(
	wire InboundGroupEventWire,
	occurredAt time.Time,
) (GroupEvent, error) {
	eventType, err := ParseGroupEventType(wire.EventType)
	if err != nil {
		return GroupEvent{}, err
	}
	details := inboundGroupEventPayload{}
	if len(wire.Plain) > 0 {
		if err := json.Unmarshal(wire.Plain, &details); err != nil {
			return GroupEvent{}, err
		}
	}
	event := GroupEvent{
		ID:          strings.TrimSpace(wire.EventID),
		GroupID:     strings.TrimSpace(wire.ConversationID),
		Version:     wire.MembershipVersion,
		Type:        eventType,
		ActorID:     strings.TrimSpace(wire.SenderID),
		OccurredAt:  occurredAt,
		MemberID:    strings.TrimSpace(details.MemberID),
		Title:       strings.TrimSpace(details.Title),
		Description: strings.TrimSpace(details.Description),
		Avatar:      strings.TrimSpace(details.Avatar),
		KeyVersion:  details.KeyVersion,
	}
	if parsedRole, err := ParseGroupMemberRole(details.Role); err == nil {
		event.Role = parsedRole
	}
	if ts := strings.TrimSpace(details.OccurredAt); ts != "" {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, ts); parseErr == nil {
			event.OccurredAt = parsed.UTC()
		}
	}
	switch event.Type {
	case GroupEventTypeMemberLeave:
		if event.MemberID == "" {
			event.MemberID = event.ActorID
		}
	case GroupEventTypeMemberAdd, GroupEventTypeMemberRemove:
		if event.MemberID == "" {
			event.MemberID = strings.TrimSpace(wire.RecipientID)
		}
		if event.Role == "" {
			event.Role = GroupMemberRoleUser
		}
	}
	if err := ValidateGroupEvent(event); err != nil {
		return GroupEvent{}, err
	}
	return event, nil
}
