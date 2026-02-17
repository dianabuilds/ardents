package policy

import groupmodel "aim-chat/go-backend/internal/domains/group/model"

type Group = groupmodel.Group
type GroupEvent = groupmodel.GroupEvent
type GroupState = groupmodel.GroupState

const (
	GroupMemberStatusInvited = groupmodel.GroupMemberStatusInvited
	GroupMemberStatusActive  = groupmodel.GroupMemberStatusActive
	GroupEventTypeMemberAdd  = groupmodel.GroupEventTypeMemberAdd
)

var (
	ErrGroupPermissionDenied            = groupmodel.ErrGroupPermissionDenied
	ErrOutOfOrderGroupEvent             = groupmodel.ErrOutOfOrderGroupEvent
	ErrGroupNotFound                    = groupmodel.ErrGroupNotFound
	ErrGroupMemberLimitExceeded         = groupmodel.ErrGroupMemberLimitExceeded
	ErrGroupPendingInvitesLimitExceeded = groupmodel.ErrGroupPendingInvitesLimitExceeded
	ErrInvalidGroupEventPayload         = groupmodel.ErrInvalidGroupEventPayload
)

func NewGroupState(group Group) GroupState {
	return groupmodel.NewGroupState(group)
}
