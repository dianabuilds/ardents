package usecase

import (
	"errors"
	"time"
)

type MembershipService struct {
	States          map[string]GroupState
	EventLog        map[string][]GroupEvent
	Persist         SnapshotPersist
	Notify          func(GroupEvent)
	GenerateEventID func() string
}

func (s *MembershipService) CreateGroup(
	title,
	identityID string,
	now time.Time,
	generateID func(prefix string) (string, error),
) (Group, GroupEvent, error) {
	title, err := NormalizeGroupTitle(title)
	if err != nil {
		return Group{}, GroupEvent{}, err
	}
	identityID, err = NormalizeGroupMemberID(identityID)
	if err != nil {
		return Group{}, GroupEvent{}, err
	}
	if generateID == nil {
		return Group{}, GroupEvent{}, errors.New("id generator is required")
	}
	if s.States == nil {
		s.States = make(map[string]GroupState)
	}
	if s.EventLog == nil {
		s.EventLog = make(map[string][]GroupEvent)
	}

	var groupID string
	for i := 0; i < 3; i++ {
		candidate, genErr := generateID("group")
		if genErr != nil {
			return Group{}, GroupEvent{}, genErr
		}
		if _, exists := s.States[candidate]; !exists {
			groupID = candidate
			break
		}
	}
	if groupID == "" {
		return Group{}, GroupEvent{}, errors.New("failed to allocate unique group id")
	}
	eventID, err := generateID("gevt")
	if err != nil {
		return Group{}, GroupEvent{}, err
	}

	group := Group{
		ID:        groupID,
		Title:     title,
		CreatedBy: identityID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	state := NewGroupState(group)
	event := GroupEvent{
		ID:         eventID,
		GroupID:    groupID,
		Version:    1,
		Type:       GroupEventTypeMemberAdd,
		ActorID:    identityID,
		OccurredAt: now,
		MemberID:   identityID,
		Role:       GroupMemberRoleOwner,
	}
	if _, err := ApplyGroupEvent(&state, event); err != nil {
		return Group{}, GroupEvent{}, err
	}
	owner := state.Members[identityID]
	owner.Status = GroupMemberStatusActive
	owner.ActivatedAt = now
	owner.UpdatedAt = now
	state.Members[identityID] = owner
	state.Group.UpdatedAt = now
	state.LastKeyVersion = 1

	s.States[groupID] = state
	s.EventLog[groupID] = []GroupEvent{event}
	if s.Persist != nil {
		if err := s.Persist(s.States, s.EventLog); err != nil {
			delete(s.States, groupID)
			delete(s.EventLog, groupID)
			return Group{}, GroupEvent{}, err
		}
	}
	if s.Notify != nil {
		s.Notify(event)
	}
	return state.Group, event, nil
}

func (s *MembershipService) InviteToGroup(
	groupID,
	actorID,
	memberID string,
	now time.Time,
	isBlockedSender func(string) bool,
	abuse *AbuseProtection,
) (GroupMember, GroupEvent, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	actorID, err = NormalizeGroupMemberID(actorID)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	memberID, err = NormalizeGroupMemberID(memberID)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	if memberID == actorID {
		return GroupMember{}, GroupEvent{}, ErrGroupCannotInviteSelf
	}
	if isBlockedSender != nil && isBlockedSender(memberID) {
		return GroupMember{}, GroupEvent{}, ErrGroupMemberBlocked
	}
	if abuse != nil && !abuse.AllowInvite(actorID, now) {
		return GroupMember{}, GroupEvent{}, ErrGroupRateLimitExceeded
	}
	state, err := LoadStateForActor(s.States, groupID, actorID, true)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	actor := state.Members[actorID]
	if !actor.CanManageMembers() {
		return GroupMember{}, GroupEvent{}, ErrGroupPermissionDenied
	}
	if existing, ok := state.Members[memberID]; ok {
		if existing.Status == GroupMemberStatusInvited || existing.Status == GroupMemberStatusActive {
			return existing, GroupEvent{}, nil
		}
	}
	if abuse != nil {
		if err := abuse.EnforceInviteQuotas(state); err != nil {
			return GroupMember{}, GroupEvent{}, err
		}
	}

	event := GroupEvent{
		ID:         s.generateEventID(),
		GroupID:    groupID,
		Version:    state.Version + 1,
		Type:       GroupEventTypeMemberAdd,
		ActorID:    actorID,
		OccurredAt: now,
		MemberID:   memberID,
		Role:       GroupMemberRoleUser,
	}
	next, err := s.applyEvent(state, event)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	member, ok := next.Members[memberID]
	if !ok {
		return GroupMember{}, GroupEvent{}, ErrGroupMembershipNotFound
	}
	return member, event, nil
}

func (s *MembershipService) LeaveGroup(groupID, actorID string, now time.Time, abuse *AbuseProtection) (bool, GroupEvent, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return false, GroupEvent{}, err
	}
	actorID, err = NormalizeGroupMemberID(actorID)
	if err != nil {
		return false, GroupEvent{}, err
	}
	if abuse != nil && !abuse.AllowMembership(actorID, now) {
		return false, GroupEvent{}, ErrGroupRateLimitExceeded
	}
	state, ok := s.States[groupID]
	if !ok {
		return false, GroupEvent{}, ErrGroupNotFound
	}
	member, exists := state.Members[actorID]
	if !exists {
		return false, GroupEvent{}, ErrGroupMembershipNotFound
	}
	if member.Status == GroupMemberStatusLeft || member.Status == GroupMemberStatusRemoved {
		return true, GroupEvent{}, nil
	}
	event := GroupEvent{
		ID:         s.generateEventID(),
		GroupID:    groupID,
		Version:    state.Version + 1,
		Type:       GroupEventTypeMemberLeave,
		ActorID:    actorID,
		OccurredAt: now,
		MemberID:   actorID,
	}
	if _, err := s.applyMembershipChangeWithKeyRotation(state, event); err != nil {
		return false, GroupEvent{}, err
	}
	return true, event, nil
}

func (s *MembershipService) AcceptGroupInvite(groupID, actorID string, now time.Time, abuse *AbuseProtection) (bool, GroupEvent, error) {
	groupID, actorID, state, member, ok, err := s.loadActorMembershipState(groupID, actorID, now, abuse)
	if err != nil {
		return false, GroupEvent{}, err
	}
	if !ok {
		return false, GroupEvent{}, ErrGroupMembershipNotFound
	}
	if member.Status == GroupMemberStatusActive {
		return true, GroupEvent{}, nil
	}
	if member.Status != GroupMemberStatusInvited {
		return false, GroupEvent{}, ErrInvalidGroupMemberState
	}
	event, err := s.applySelfMembershipChange(state, groupID, actorID, now, GroupEventTypeMemberAdd, member.Role)
	if err != nil {
		return false, GroupEvent{}, err
	}
	return true, event, nil
}

func (s *MembershipService) DeclineGroupInvite(groupID, actorID string, now time.Time, abuse *AbuseProtection) (bool, GroupEvent, error) {
	groupID, actorID, state, member, ok, err := s.loadActorMembershipState(groupID, actorID, now, abuse)
	if err != nil {
		return false, GroupEvent{}, err
	}
	if !ok || member.Status == GroupMemberStatusRemoved {
		return true, GroupEvent{}, nil
	}
	if member.Status != GroupMemberStatusInvited {
		return false, GroupEvent{}, ErrInvalidGroupMemberState
	}
	event, err := s.applySelfMembershipChange(state, groupID, actorID, now, GroupEventTypeMemberRemove, "")
	if err != nil {
		return false, GroupEvent{}, err
	}
	return true, event, nil
}

func (s *MembershipService) applySelfMembershipChange(
	state GroupState,
	groupID, actorID string,
	now time.Time,
	eventType GroupEventType,
	role GroupMemberRole,
) (GroupEvent, error) {
	event := GroupEvent{
		ID:         s.generateEventID(),
		GroupID:    groupID,
		Version:    state.Version + 1,
		Type:       eventType,
		ActorID:    actorID,
		OccurredAt: now,
		MemberID:   actorID,
	}
	if eventType == GroupEventTypeMemberAdd {
		event.Role = role
	}
	if _, err := s.applyMembershipChangeWithKeyRotation(state, event); err != nil {
		return GroupEvent{}, err
	}
	return event, nil
}

func (s *MembershipService) loadActorMembershipState(
	groupID, actorID string,
	now time.Time,
	abuse *AbuseProtection,
) (string, string, GroupState, GroupMember, bool, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return "", "", GroupState{}, GroupMember{}, false, err
	}
	actorID, err = NormalizeGroupMemberID(actorID)
	if err != nil {
		return "", "", GroupState{}, GroupMember{}, false, err
	}
	if abuse != nil && !abuse.AllowMembership(actorID, now) {
		return "", "", GroupState{}, GroupMember{}, false, ErrGroupRateLimitExceeded
	}
	state, err := LoadStateForActor(s.States, groupID, actorID, false)
	if err != nil {
		return "", "", GroupState{}, GroupMember{}, false, err
	}
	member, ok := state.Members[actorID]
	return groupID, actorID, state, member, ok, nil
}

func (s *MembershipService) RemoveGroupMember(groupID, actorID, memberID string, now time.Time, abuse *AbuseProtection) (bool, GroupEvent, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return false, GroupEvent{}, err
	}
	actorID, err = NormalizeGroupMemberID(actorID)
	if err != nil {
		return false, GroupEvent{}, err
	}
	memberID, err = NormalizeGroupMemberID(memberID)
	if err != nil {
		return false, GroupEvent{}, err
	}
	if abuse != nil && !abuse.AllowMembership(actorID, now) {
		return false, GroupEvent{}, ErrGroupRateLimitExceeded
	}
	state, err := LoadStateForActor(s.States, groupID, actorID, true)
	if err != nil {
		return false, GroupEvent{}, err
	}
	actor := state.Members[actorID]
	if !actor.CanManageMembers() {
		return false, GroupEvent{}, ErrGroupPermissionDenied
	}
	target, exists := state.Members[memberID]
	if !exists {
		return false, GroupEvent{}, ErrGroupMembershipNotFound
	}
	if target.IsOwner() {
		return false, GroupEvent{}, ErrGroupPermissionDenied
	}
	if target.Status == GroupMemberStatusRemoved {
		return true, GroupEvent{}, nil
	}
	event := GroupEvent{
		ID:         s.generateEventID(),
		GroupID:    groupID,
		Version:    state.Version + 1,
		Type:       GroupEventTypeMemberRemove,
		ActorID:    actorID,
		OccurredAt: now,
		MemberID:   memberID,
	}
	if _, err := s.applyMembershipChangeWithKeyRotation(state, event); err != nil {
		return false, GroupEvent{}, err
	}
	return true, event, nil
}

func (s *MembershipService) ChangeGroupMemberRole(
	groupID,
	actorID,
	memberID string,
	role GroupMemberRole,
	now time.Time,
	abuse *AbuseProtection,
) (GroupMember, GroupEvent, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	actorID, err = NormalizeGroupMemberID(actorID)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	memberID, err = NormalizeGroupMemberID(memberID)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	if abuse != nil && !abuse.AllowMembership(actorID, now) {
		return GroupMember{}, GroupEvent{}, ErrGroupRateLimitExceeded
	}
	state, err := LoadStateForActor(s.States, groupID, actorID, true)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	actor := state.Members[actorID]
	if !actor.IsOwner() {
		return GroupMember{}, GroupEvent{}, ErrGroupPermissionDenied
	}
	target, exists := state.Members[memberID]
	if !exists {
		return GroupMember{}, GroupEvent{}, ErrGroupMembershipNotFound
	}
	if target.IsOwner() {
		return GroupMember{}, GroupEvent{}, ErrGroupPermissionDenied
	}
	if !target.CanMutateRole() {
		return GroupMember{}, GroupEvent{}, ErrInvalidGroupMemberState
	}
	if target.Role == role {
		return target, GroupEvent{}, nil
	}
	event := GroupEvent{
		ID:         s.generateEventID(),
		GroupID:    groupID,
		Version:    state.Version + 1,
		Type:       GroupEventTypeMemberAdd,
		ActorID:    actorID,
		OccurredAt: now,
		MemberID:   memberID,
		Role:       role,
	}
	next, err := s.applyEvent(state, event)
	if err != nil {
		return GroupMember{}, GroupEvent{}, err
	}
	member, ok := next.Members[memberID]
	if !ok {
		return GroupMember{}, GroupEvent{}, ErrGroupMembershipNotFound
	}
	return member, event, nil
}

func (s *MembershipService) generateEventID() string {
	if s.GenerateEventID == nil {
		return "gevt_fallback"
	}
	return s.GenerateEventID()
}

func (s *MembershipService) applyEvent(state GroupState, event GroupEvent) (GroupState, error) {
	return s.applyEvents(state, event)
}

func (s *MembershipService) applyMembershipChangeWithKeyRotation(state GroupState, change GroupEvent) (GroupState, error) {
	nextKeyVersion := state.LastKeyVersion + 1
	if state.LastKeyVersion == 0 {
		nextKeyVersion = 1
	}
	rotate := GroupEvent{
		ID:         s.generateEventID(),
		GroupID:    change.GroupID,
		Version:    change.Version + 1,
		Type:       GroupEventTypeKeyRotate,
		ActorID:    change.ActorID,
		OccurredAt: change.OccurredAt,
		KeyVersion: nextKeyVersion,
	}
	return s.applyEvents(state, change, rotate)
}

func (s *MembershipService) applyEvents(state GroupState, events ...GroupEvent) (GroupState, error) {
	next, applied, err := ApplyEventsWithRollback(state, s.States, s.EventLog, s.Persist, events...)
	if err != nil {
		return GroupState{}, err
	}
	if s.Notify != nil {
		for _, event := range applied {
			s.Notify(event)
		}
	}
	return next, nil
}
