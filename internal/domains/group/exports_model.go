//goland:noinspection GoNameStartsWithPackageName
package group

import groupmodel "aim-chat/go-backend/internal/domains/group/model"

//goland:noinspection GoNameStartsWithPackageName
type GroupMemberRole = groupmodel.GroupMemberRole

//goland:noinspection GoNameStartsWithPackageName
const (
	GroupMemberRoleOwner = groupmodel.GroupMemberRoleOwner
	GroupMemberRoleAdmin = groupmodel.GroupMemberRoleAdmin
	GroupMemberRoleUser  = groupmodel.GroupMemberRoleUser
)

//goland:noinspection GoNameStartsWithPackageName
type GroupMemberStatus = groupmodel.GroupMemberStatus

//goland:noinspection GoNameStartsWithPackageName
const (
	GroupMemberStatusInvited = groupmodel.GroupMemberStatusInvited
	GroupMemberStatusActive  = groupmodel.GroupMemberStatusActive
	GroupMemberStatusLeft    = groupmodel.GroupMemberStatusLeft
	GroupMemberStatusRemoved = groupmodel.GroupMemberStatusRemoved
)

var (
	ErrInvalidGroupID                     = groupmodel.ErrInvalidGroupID
	ErrInvalidGroupMemberID               = groupmodel.ErrInvalidGroupMemberID
	ErrInvalidGroupMemberRole             = groupmodel.ErrInvalidGroupMemberRole
	ErrInvalidGroupMemberStatus           = groupmodel.ErrInvalidGroupMemberStatus
	ErrInvalidGroupMemberStatusTransition = groupmodel.ErrInvalidGroupMemberStatusTransition
	ErrInvalidGroupEventID                = groupmodel.ErrInvalidGroupEventID
	ErrInvalidGroupEventType              = groupmodel.ErrInvalidGroupEventType
	ErrInvalidGroupEventVersion           = groupmodel.ErrInvalidGroupEventVersion
	ErrInvalidGroupEventActorID           = groupmodel.ErrInvalidGroupEventActorID
	ErrInvalidGroupEventPayload           = groupmodel.ErrInvalidGroupEventPayload
	ErrOutOfOrderGroupEvent               = groupmodel.ErrOutOfOrderGroupEvent
)

//goland:noinspection GoNameStartsWithPackageName
type Group = groupmodel.Group

//goland:noinspection GoNameStartsWithPackageName
type GroupMember = groupmodel.GroupMember

func ParseGroupMemberRole(raw string) (GroupMemberRole, error) {
	return groupmodel.ParseGroupMemberRole(raw)
}

func ParseGroupMemberStatus(raw string) (GroupMemberStatus, error) {
	return groupmodel.ParseGroupMemberStatus(raw)
}

func ValidateGroupMember(member GroupMember) error {
	return groupmodel.ValidateGroupMember(member)
}

func ValidateGroupMemberStatusTransition(from, to GroupMemberStatus) error {
	return groupmodel.ValidateGroupMemberStatusTransition(from, to)
}

//goland:noinspection GoNameStartsWithPackageName
type GroupEventType = groupmodel.GroupEventType

//goland:noinspection GoNameStartsWithPackageName
const (
	GroupEventTypeMemberAdd    = groupmodel.GroupEventTypeMemberAdd
	GroupEventTypeMemberRemove = groupmodel.GroupEventTypeMemberRemove
	GroupEventTypeMemberLeave  = groupmodel.GroupEventTypeMemberLeave
	GroupEventTypeTitleChange  = groupmodel.GroupEventTypeTitleChange
	GroupEventTypeKeyRotate    = groupmodel.GroupEventTypeKeyRotate
)

//goland:noinspection GoNameStartsWithPackageName
type GroupEvent = groupmodel.GroupEvent

//goland:noinspection GoNameStartsWithPackageName
type GroupState = groupmodel.GroupState

func NewGroupState(group Group) GroupState {
	return groupmodel.NewGroupState(group)
}

func ParseGroupEventType(raw string) (GroupEventType, error) {
	return groupmodel.ParseGroupEventType(raw)
}

func ValidateGroupEvent(event GroupEvent) error {
	return groupmodel.ValidateGroupEvent(event)
}

func ApplyGroupEvent(state *GroupState, event GroupEvent) (bool, error) {
	return groupmodel.ApplyGroupEvent(state, event)
}

//goland:noinspection GoNameStartsWithPackageName
type GroupMessageRecipientStatus = groupmodel.GroupMessageRecipientStatus

//goland:noinspection GoNameStartsWithPackageName
type GroupMessageFanoutResult = groupmodel.GroupMessageFanoutResult
