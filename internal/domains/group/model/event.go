package model

import (
	"errors"
	"strings"
	"time"
)

type GroupEventType string

const (
	GroupEventTypeMemberAdd     GroupEventType = "member_add"
	GroupEventTypeMemberRemove  GroupEventType = "member_remove"
	GroupEventTypeMemberLeave   GroupEventType = "member_leave"
	GroupEventTypeTitleChange   GroupEventType = "title_change"
	GroupEventTypeProfileChange GroupEventType = "profile_change"
	GroupEventTypeKeyRotate     GroupEventType = "key_rotate"
)

var (
	ErrInvalidGroupEventID      = errors.New("invalid group event id")
	ErrInvalidGroupEventType    = errors.New("invalid group event type")
	ErrInvalidGroupEventVersion = errors.New("invalid group event version")
	ErrInvalidGroupEventActorID = errors.New("invalid group event actor id")
	ErrInvalidGroupEventPayload = errors.New("invalid group event payload")
	ErrOutOfOrderGroupEvent     = errors.New("out-of-order group event")
)

// GroupEvent is a versioned event for group lifecycle changes.
type GroupEvent struct {
	ID         string         `json:"id"`
	GroupID    string         `json:"group_id"`
	Version    uint64         `json:"version"`
	Type       GroupEventType `json:"type"`
	ActorID    string         `json:"actor_id"`
	OccurredAt time.Time      `json:"occurred_at"`

	MemberID    string          `json:"member_id,omitempty"`
	Role        GroupMemberRole `json:"role,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Avatar      string          `json:"avatar,omitempty"`

	KeyVersion uint32 `json:"key_version,omitempty"`
}

// GroupState is an in-memory event-application state used by domain flows.
type GroupState struct {
	Group           Group                  `json:"group"`
	Version         uint64                 `json:"version"`
	AppliedEventIDs map[string]struct{}    `json:"applied_event_ids"`
	Members         map[string]GroupMember `json:"members"`
	LastKeyVersion  uint32                 `json:"last_key_version"`
}

func NewGroupState(group Group) GroupState {
	return GroupState{
		Group:           group,
		Version:         0,
		AppliedEventIDs: make(map[string]struct{}),
		Members:         make(map[string]GroupMember),
	}
}

func (t GroupEventType) Valid() bool {
	switch t {
	case GroupEventTypeMemberAdd, GroupEventTypeMemberRemove, GroupEventTypeMemberLeave, GroupEventTypeTitleChange, GroupEventTypeProfileChange, GroupEventTypeKeyRotate:
		return true
	default:
		return false
	}
}

func ParseGroupEventType(raw string) (GroupEventType, error) {
	typ := GroupEventType(strings.TrimSpace(raw))
	if !typ.Valid() {
		return "", ErrInvalidGroupEventType
	}
	return typ, nil
}

func ValidateGroupEvent(event GroupEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		return ErrInvalidGroupEventID
	}
	if strings.TrimSpace(event.GroupID) == "" {
		return ErrInvalidGroupID
	}
	if event.Version == 0 {
		return ErrInvalidGroupEventVersion
	}
	if !event.Type.Valid() {
		return ErrInvalidGroupEventType
	}
	if strings.TrimSpace(event.ActorID) == "" {
		return ErrInvalidGroupEventActorID
	}
	if event.OccurredAt.IsZero() {
		return ErrInvalidGroupEventPayload
	}

	switch event.Type {
	case GroupEventTypeMemberAdd:
		if strings.TrimSpace(event.MemberID) == "" {
			return ErrInvalidGroupEventPayload
		}
		if !event.Role.Valid() {
			return ErrInvalidGroupEventPayload
		}
	case GroupEventTypeMemberRemove:
		if strings.TrimSpace(event.MemberID) == "" {
			return ErrInvalidGroupEventPayload
		}
	case GroupEventTypeMemberLeave:
		if strings.TrimSpace(event.MemberID) == "" {
			return ErrInvalidGroupEventPayload
		}
	case GroupEventTypeTitleChange:
		if strings.TrimSpace(event.Title) == "" {
			return ErrInvalidGroupEventPayload
		}
	case GroupEventTypeProfileChange:
		if strings.TrimSpace(event.Title) == "" {
			return ErrInvalidGroupEventPayload
		}
	case GroupEventTypeKeyRotate:
		if event.KeyVersion == 0 {
			return ErrInvalidGroupEventPayload
		}
	}
	return nil
}

// ApplyGroupEvent applies a validated event to state.
// Returns applied=false when same event ID is re-applied (idempotent no-op).
func ApplyGroupEvent(state *GroupState, event GroupEvent) (bool, error) {
	if state == nil {
		return false, ErrInvalidGroupEventPayload
	}
	if err := ValidateGroupEvent(event); err != nil {
		return false, err
	}
	if strings.TrimSpace(state.Group.ID) == "" {
		return false, ErrInvalidGroupID
	}
	if event.GroupID != state.Group.ID {
		return false, ErrInvalidGroupID
	}
	if state.AppliedEventIDs == nil {
		state.AppliedEventIDs = make(map[string]struct{})
	}
	if state.Members == nil {
		state.Members = make(map[string]GroupMember)
	}

	if _, exists := state.AppliedEventIDs[event.ID]; exists {
		return false, nil
	}

	expected := state.Version + 1
	if event.Version != expected {
		return false, ErrOutOfOrderGroupEvent
	}

	switch event.Type {
	case GroupEventTypeMemberAdd:
		memberID := strings.TrimSpace(event.MemberID)
		member, exists := state.Members[memberID]
		if !exists {
			member = GroupMember{
				GroupID:   state.Group.ID,
				MemberID:  memberID,
				Role:      event.Role,
				Status:    GroupMemberStatusInvited,
				InvitedAt: event.OccurredAt.UTC(),
				UpdatedAt: event.OccurredAt.UTC(),
			}
			state.Members[member.MemberID] = member
			break
		}
		member.Role = event.Role
		switch member.Status {
		case GroupMemberStatusInvited:
			if strings.TrimSpace(event.ActorID) == memberID {
				member.Status = GroupMemberStatusActive
				member.ActivatedAt = event.OccurredAt.UTC()
			}
		case GroupMemberStatusLeft, GroupMemberStatusRemoved:
			member.Status = GroupMemberStatusInvited
			member.InvitedAt = event.OccurredAt.UTC()
			member.ActivatedAt = time.Time{}
		}
		member.UpdatedAt = event.OccurredAt.UTC()
		state.Members[member.MemberID] = member
	case GroupEventTypeMemberRemove:
		memberID := strings.TrimSpace(event.MemberID)
		member, ok := state.Members[memberID]
		if !ok {
			member = GroupMember{
				GroupID:  state.Group.ID,
				MemberID: memberID,
				Role:     GroupMemberRoleUser,
			}
		}
		member.Status = GroupMemberStatusRemoved
		member.UpdatedAt = event.OccurredAt.UTC()
		state.Members[memberID] = member
	case GroupEventTypeMemberLeave:
		memberID := strings.TrimSpace(event.MemberID)
		member, ok := state.Members[memberID]
		if !ok {
			member = GroupMember{
				GroupID:  state.Group.ID,
				MemberID: memberID,
				Role:     GroupMemberRoleUser,
			}
		}
		member.Status = GroupMemberStatusLeft
		member.UpdatedAt = event.OccurredAt.UTC()
		state.Members[memberID] = member
	case GroupEventTypeTitleChange:
		state.Group.Title = strings.TrimSpace(event.Title)
		state.Group.UpdatedAt = event.OccurredAt.UTC()
	case GroupEventTypeProfileChange:
		state.Group.Title = strings.TrimSpace(event.Title)
		state.Group.Description = strings.TrimSpace(event.Description)
		state.Group.Avatar = strings.TrimSpace(event.Avatar)
		state.Group.UpdatedAt = event.OccurredAt.UTC()
	case GroupEventTypeKeyRotate:
		state.LastKeyVersion = event.KeyVersion
		state.Group.UpdatedAt = event.OccurredAt.UTC()
	}

	state.Version = event.Version
	state.AppliedEventIDs[event.ID] = struct{}{}
	return true, nil
}
