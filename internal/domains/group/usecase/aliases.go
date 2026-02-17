package usecase

import (
	groupmodel "aim-chat/go-backend/internal/domains/group/model"
	grouppolicy "aim-chat/go-backend/internal/domains/group/policy"
)

type Group = groupmodel.Group
type GroupMember = groupmodel.GroupMember
type GroupState = groupmodel.GroupState
type GroupEvent = groupmodel.GroupEvent
type GroupEventType = groupmodel.GroupEventType
type GroupMemberRole = groupmodel.GroupMemberRole
type GroupMemberStatus = groupmodel.GroupMemberStatus
type GroupMessageRecipientStatus = groupmodel.GroupMessageRecipientStatus
type GroupMessageFanoutResult = groupmodel.GroupMessageFanoutResult

const (
	GroupEventTypeMemberAdd    = groupmodel.GroupEventTypeMemberAdd
	GroupEventTypeMemberRemove = groupmodel.GroupEventTypeMemberRemove
	GroupEventTypeMemberLeave  = groupmodel.GroupEventTypeMemberLeave
	GroupEventTypeTitleChange  = groupmodel.GroupEventTypeTitleChange
	GroupEventTypeKeyRotate    = groupmodel.GroupEventTypeKeyRotate
)

const (
	GroupMemberRoleOwner = groupmodel.GroupMemberRoleOwner
	GroupMemberRoleAdmin = groupmodel.GroupMemberRoleAdmin
	GroupMemberRoleUser  = groupmodel.GroupMemberRoleUser
)

const (
	GroupMemberStatusInvited = groupmodel.GroupMemberStatusInvited
	GroupMemberStatusActive  = groupmodel.GroupMemberStatusActive
	GroupMemberStatusLeft    = groupmodel.GroupMemberStatusLeft
	GroupMemberStatusRemoved = groupmodel.GroupMemberStatusRemoved
)

var (
	ErrInvalidGroupMemberID       = groupmodel.ErrInvalidGroupMemberID
	ErrInvalidGroupEventPayload   = groupmodel.ErrInvalidGroupEventPayload
	ErrGroupNotFound              = groupmodel.ErrGroupNotFound
	ErrGroupMembershipNotFound    = groupmodel.ErrGroupMembershipNotFound
	ErrGroupPermissionDenied      = groupmodel.ErrGroupPermissionDenied
	ErrGroupCannotInviteSelf      = groupmodel.ErrGroupCannotInviteSelf
	ErrGroupMemberBlocked         = groupmodel.ErrGroupMemberBlocked
	ErrGroupSenderBlocked         = groupmodel.ErrGroupSenderBlocked
	ErrInvalidGroupMemberState    = groupmodel.ErrInvalidGroupMemberState
	ErrGroupRateLimitExceeded     = groupmodel.ErrGroupRateLimitExceeded
	ErrInvalidGroupMessageContent = groupmodel.ErrInvalidGroupMessageContent
)

func NormalizeGroupID(groupID string) (string, error) {
	return groupmodel.NormalizeGroupID(groupID)
}

func NormalizeGroupTitle(title string) (string, error) {
	return groupmodel.NormalizeGroupTitle(title)
}

func ParseGroupEventType(raw string) (GroupEventType, error) {
	return groupmodel.ParseGroupEventType(raw)
}

func ParseGroupMemberRole(raw string) (GroupMemberRole, error) {
	return groupmodel.ParseGroupMemberRole(raw)
}

func ValidateGroupEvent(event GroupEvent) error {
	return groupmodel.ValidateGroupEvent(event)
}

func NewGroupState(group Group) GroupState {
	return groupmodel.NewGroupState(group)
}

func ApplyGroupEvent(state *GroupState, event GroupEvent) (bool, error) {
	return groupmodel.ApplyGroupEvent(state, event)
}

type AbuseProtection = grouppolicy.AbuseProtection

type InboundGroupMessageRejectReason = grouppolicy.InboundGroupMessageRejectReason

const (
	InboundGroupMessageReasonMembershipVersionMismatch = grouppolicy.InboundGroupMessageReasonMembershipVersionMismatch
	InboundGroupMessageReasonGroupKeyVersionMismatch   = grouppolicy.InboundGroupMessageReasonGroupKeyVersionMismatch
)

func ValidateInboundGroupMessageState(
	state GroupState,
	senderID string,
	membershipVersion uint64,
	groupKeyVersion uint32,
) (InboundGroupMessageRejectReason, error) {
	return grouppolicy.ValidateInboundGroupMessageState(state, senderID, membershipVersion, groupKeyVersion)
}

func EnsureInboundEventState(
	states map[string]GroupState,
	event GroupEvent,
	localIdentityID string,
) (GroupState, error) {
	return grouppolicy.EnsureInboundEventState(states, event, localIdentityID)
}

func DeriveRecipientMessageID(eventID, recipientID string) string {
	return grouppolicy.DeriveRecipientMessageID(eventID, recipientID)
}

func CorrelationID(groupID, eventID string) string {
	return grouppolicy.CorrelationID(groupID, eventID)
}
