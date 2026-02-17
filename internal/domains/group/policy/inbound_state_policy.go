package policy

import (
	"strings"
)

type InboundGroupMessageRejectReason string

const (
	InboundGroupMessageReasonUnauthorizedSender        InboundGroupMessageRejectReason = "unauthorized_sender"
	InboundGroupMessageReasonMembershipVersionMismatch InboundGroupMessageRejectReason = "membership_version_mismatch"
	InboundGroupMessageReasonGroupKeyVersionMismatch   InboundGroupMessageRejectReason = "group_key_version_mismatch"
)

func ValidateInboundGroupMessageState(
	state GroupState,
	senderID string,
	membershipVersion uint64,
	groupKeyVersion uint32,
) (InboundGroupMessageRejectReason, error) {
	member, memberExists := state.Members[senderID]
	if !memberExists || member.Status != GroupMemberStatusActive {
		return InboundGroupMessageReasonUnauthorizedSender, ErrGroupPermissionDenied
	}
	if membershipVersion != state.Version {
		return InboundGroupMessageReasonMembershipVersionMismatch, ErrOutOfOrderGroupEvent
	}
	expectedGroupKeyVersion := state.LastKeyVersion
	if expectedGroupKeyVersion == 0 {
		expectedGroupKeyVersion = 1
	}
	if groupKeyVersion != expectedGroupKeyVersion {
		return InboundGroupMessageReasonGroupKeyVersionMismatch, ErrOutOfOrderGroupEvent
	}
	return "", nil
}

func EnsureInboundEventState(
	states map[string]GroupState,
	event GroupEvent,
	localIdentityID string,
) (GroupState, error) {
	if state, ok := states[event.GroupID]; ok {
		return state, nil
	}
	localIdentityID = strings.TrimSpace(localIdentityID)
	if event.Type != GroupEventTypeMemberAdd || event.Version != 1 || event.MemberID != localIdentityID {
		return GroupState{}, ErrGroupNotFound
	}
	bootstrapGroup := Group{
		ID:        event.GroupID,
		Title:     event.GroupID,
		CreatedBy: event.ActorID,
		CreatedAt: event.OccurredAt,
		UpdatedAt: event.OccurredAt,
	}
	return NewGroupState(bootstrapGroup), nil
}
