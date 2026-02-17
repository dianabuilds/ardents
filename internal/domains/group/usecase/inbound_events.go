package usecase

import (
	"encoding/json"
	"strings"
	"time"
)

type inboundGroupEventPayload struct {
	MemberID   string `json:"member_id"`
	Role       string `json:"role"`
	Title      string `json:"title"`
	KeyVersion uint32 `json:"key_version"`
	OccurredAt string `json:"occurred_at"`
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
		ID:         strings.TrimSpace(wire.EventID),
		GroupID:    strings.TrimSpace(wire.ConversationID),
		Version:    wire.MembershipVersion,
		Type:       eventType,
		ActorID:    strings.TrimSpace(wire.SenderID),
		OccurredAt: occurredAt,
		MemberID:   strings.TrimSpace(details.MemberID),
		Title:      strings.TrimSpace(details.Title),
		KeyVersion: details.KeyVersion,
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

func AuthorizeInboundGroupEvent(state GroupState, event GroupEvent) error {
	actor, actorExists := state.Members[event.ActorID]
	switch event.Type {
	case GroupEventTypeMemberAdd:
		target, targetExists := state.Members[event.MemberID]
		if !actorExists {
			// bootstrap invite/initial add is handled by caller for unknown groups
			return ErrGroupPermissionDenied
		}
		if targetExists && target.Status == GroupMemberStatusInvited && event.ActorID == event.MemberID {
			return nil
		}
		if actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		if !targetExists {
			if actor.Role == GroupMemberRoleOwner || actor.Role == GroupMemberRoleAdmin {
				return nil
			}
			return ErrGroupPermissionDenied
		}
		// Role changes are owner-only.
		if target.Role != event.Role {
			if actor.Role != GroupMemberRoleOwner {
				return ErrGroupPermissionDenied
			}
			if target.Role == GroupMemberRoleOwner {
				return ErrGroupPermissionDenied
			}
		}
		return nil
	case GroupEventTypeMemberRemove:
		if !actorExists || actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		target, targetExists := state.Members[event.MemberID]
		if !targetExists {
			return ErrGroupMembershipNotFound
		}
		if actor.Role != GroupMemberRoleOwner && actor.Role != GroupMemberRoleAdmin {
			return ErrGroupPermissionDenied
		}
		if target.Role == GroupMemberRoleOwner {
			return ErrGroupPermissionDenied
		}
		if actor.Role == GroupMemberRoleAdmin && target.Role == GroupMemberRoleAdmin {
			return ErrGroupPermissionDenied
		}
		return nil
	case GroupEventTypeMemberLeave:
		if !actorExists {
			return ErrGroupPermissionDenied
		}
		if event.ActorID != event.MemberID {
			return ErrGroupPermissionDenied
		}
		return nil
	case GroupEventTypeTitleChange:
		if !actorExists || actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		if actor.Role != GroupMemberRoleOwner && actor.Role != GroupMemberRoleAdmin {
			return ErrGroupPermissionDenied
		}
		return nil
	case GroupEventTypeKeyRotate:
		if !actorExists || actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		if actor.Role != GroupMemberRoleOwner {
			return ErrGroupPermissionDenied
		}
		return nil
	default:
		return ErrGroupPermissionDenied
	}
}
