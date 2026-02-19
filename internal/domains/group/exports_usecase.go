package group

import groupusecase "aim-chat/go-backend/internal/domains/group/usecase"

type SnapshotPersist = groupusecase.SnapshotPersist

type Service = groupusecase.Service
type MembershipService = groupusecase.MembershipService

//goland:noinspection GoNameStartsWithPackageName
type GroupReadService = groupusecase.GroupReadService

//goland:noinspection GoNameStartsWithPackageName
type GroupMessageWireMeta = groupusecase.GroupMessageWireMeta

//goland:noinspection GoNameStartsWithPackageName
type GroupMessageFanoutService = groupusecase.GroupMessageFanoutService
type InboundGroupMessageParams = groupusecase.InboundGroupMessageParams
type InboundGroupEventParams = groupusecase.InboundGroupEventParams
type InboundOrchestrationService = groupusecase.InboundOrchestrationService

func CloneState(in GroupState) GroupState {
	return groupusecase.CloneState(in)
}
