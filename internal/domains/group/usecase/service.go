package usecase

import (
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/pkg/models"
	"strings"
	"time"
)

type Service struct {
	IdentityID func() string

	WithMembership func(fn func(svc *MembershipService) error) error
	SnapshotStates func() map[string]GroupState

	GenerateID      func(prefix string) (string, error)
	GenerateEventID func() string
	Now             func() time.Time

	Abuse           *AbuseProtection
	IsBlockedSender func(string) bool

	ActiveDeviceID       func() (string, error)
	GetMessage           func(string) (models.Message, bool)
	SaveMessage          func(models.Message) error
	DeleteMessage        func(contactID, messageID string) (bool, error)
	ListMessages         func(conversationID, conversationType string, limit, offset int) []models.Message
	ListMessagesByThread func(conversationID, conversationType, threadID string, limit, offset int) []models.Message

	PrepareAndPublish func(msg models.Message, recipientID string, meta GroupMessageWireMeta) (string, string, error)
	RecordError       func(category string, err error)
	Notify            func(method string, payload any)
	RecordAggregate   func(string)
	LogInfo           func(message string, args ...any)
}

func (s *Service) nowUTC() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s *Service) actorID() string {
	if s.IdentityID == nil {
		return ""
	}
	return strings.TrimSpace(s.IdentityID())
}

func (s *Service) logInfo(message string, args ...any) {
	if s.LogInfo != nil {
		s.LogInfo(message, args...)
	}
}

func (s *Service) recordAggregate(name string) {
	if s.RecordAggregate != nil {
		s.RecordAggregate(name)
	}
}

func (s *Service) CreateGroup(title string) (Group, error) {
	var (
		group Group
		event GroupEvent
	)
	err := s.WithMembership(func(ms *MembershipService) error {
		var err error
		group, event, err = ms.CreateGroup(title, s.actorID(), s.nowUTC(), s.GenerateID)
		return err
	})
	if err != nil {
		return Group{}, err
	}
	s.recordAggregate("create")
	s.logInfo(
		"group created",
		"correlation_id", CorrelationID(group.ID, event.ID),
		"group_id", group.ID,
		"actor_id", s.actorID(),
	)
	return group, nil
}

func (s *Service) GetGroup(groupID string) (Group, error) {
	read := &GroupReadService{States: s.SnapshotStates()}
	return read.GetGroup(groupID)
}

func (s *Service) ListGroups() ([]Group, error) {
	read := &GroupReadService{States: s.SnapshotStates()}
	return read.ListGroups()
}

func (s *Service) ListGroupMembers(groupID string) ([]GroupMember, error) {
	read := &GroupReadService{States: s.SnapshotStates()}
	return read.ListGroupMembers(groupID)
}

func (s *Service) LeaveGroup(groupID string) (bool, error) {
	var ok bool
	err := s.WithMembership(func(ms *MembershipService) error {
		var err error
		ok, _, err = ms.LeaveGroup(groupID, s.actorID(), s.nowUTC(), s.Abuse)
		return err
	})
	return ok, err
}

func (s *Service) InviteToGroup(groupID, memberID string) (GroupMember, error) {
	normalizedMemberID, err := privacydomain.NormalizeIdentityID(memberID)
	if err != nil {
		return GroupMember{}, err
	}
	var (
		member GroupMember
		event  GroupEvent
	)
	err = s.WithMembership(func(ms *MembershipService) error {
		var err error
		member, event, err = ms.InviteToGroup(groupID, s.actorID(), normalizedMemberID, s.nowUTC(), s.IsBlockedSender, s.Abuse)
		return err
	})
	if err != nil {
		return GroupMember{}, err
	}
	if strings.TrimSpace(event.ID) != "" {
		s.recordAggregate("invite")
		s.logInfo(
			"group invite applied",
			"correlation_id", CorrelationID(groupID, event.ID),
			"group_id", groupID,
			"actor_id", s.actorID(),
			"member_id", normalizedMemberID,
		)
	}
	return member, nil
}

func (s *Service) AcceptGroupInvite(groupID string) (bool, error) {
	var (
		ok    bool
		event GroupEvent
	)
	err := s.WithMembership(func(ms *MembershipService) error {
		var err error
		ok, event, err = ms.AcceptGroupInvite(groupID, s.actorID(), s.nowUTC(), s.Abuse)
		return err
	})
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(event.ID) != "" {
		s.recordAggregate("accept")
		s.logInfo(
			"group invite accepted",
			"correlation_id", CorrelationID(groupID, event.ID),
			"group_id", groupID,
			"actor_id", s.actorID(),
		)
	}
	return ok, nil
}

func (s *Service) DeclineGroupInvite(groupID string) (bool, error) {
	var ok bool
	err := s.WithMembership(func(ms *MembershipService) error {
		var err error
		ok, _, err = ms.DeclineGroupInvite(groupID, s.actorID(), s.nowUTC(), s.Abuse)
		return err
	})
	return ok, err
}

func (s *Service) RemoveGroupMember(groupID, memberID string) (bool, error) {
	normalizedMemberID, err := privacydomain.NormalizeIdentityID(memberID)
	if err != nil {
		return false, err
	}
	var (
		ok    bool
		event GroupEvent
	)
	err = s.WithMembership(func(ms *MembershipService) error {
		var err error
		ok, event, err = ms.RemoveGroupMember(groupID, s.actorID(), normalizedMemberID, s.nowUTC(), s.Abuse)
		return err
	})
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(event.ID) != "" {
		s.recordAggregate("remove")
		s.logInfo(
			"group member removed",
			"correlation_id", CorrelationID(groupID, event.ID),
			"group_id", groupID,
			"actor_id", s.actorID(),
			"member_id", normalizedMemberID,
		)
	}
	return ok, nil
}

func (s *Service) PromoteGroupMember(groupID, memberID string) (GroupMember, error) {
	return s.changeGroupMemberRole(groupID, memberID, GroupMemberRoleAdmin)
}

func (s *Service) DemoteGroupMember(groupID, memberID string) (GroupMember, error) {
	return s.changeGroupMemberRole(groupID, memberID, GroupMemberRoleUser)
}

func (s *Service) changeGroupMemberRole(groupID, memberID string, role GroupMemberRole) (GroupMember, error) {
	normalizedMemberID, err := privacydomain.NormalizeIdentityID(memberID)
	if err != nil {
		return GroupMember{}, err
	}
	var member GroupMember
	err = s.WithMembership(func(ms *MembershipService) error {
		var err error
		member, _, err = ms.ChangeGroupMemberRole(groupID, s.actorID(), normalizedMemberID, role, s.nowUTC(), s.Abuse)
		return err
	})
	return member, err
}

func (s *Service) SendGroupMessage(groupID, content string) (GroupMessageFanoutResult, error) {
	return s.sendGroupMessageWithThread(groupID, content, "")
}

func (s *Service) SendGroupMessageInThread(groupID, content, threadID string) (GroupMessageFanoutResult, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return GroupMessageFanoutResult{}, ErrInvalidGroupMessageContent
	}
	return s.sendGroupMessageWithThread(groupID, content, threadID)
}

func (s *Service) sendGroupMessageWithThread(groupID, content, threadID string) (GroupMessageFanoutResult, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return GroupMessageFanoutResult{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return GroupMessageFanoutResult{}, ErrInvalidGroupMessageContent
	}
	if s.GenerateID == nil {
		return GroupMessageFanoutResult{}, ErrInvalidGroupMessageContent
	}
	eventID, err := s.GenerateID("gevtmsg")
	if err != nil {
		return GroupMessageFanoutResult{}, err
	}
	return s.SendGroupMessageFanout(groupID, eventID, content, threadID)
}

func (s *Service) SendGroupMessageFanout(groupID, eventID, content, threadID string) (GroupMessageFanoutResult, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return GroupMessageFanoutResult{}, err
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		if s.GenerateID == nil {
			return GroupMessageFanoutResult{}, ErrInvalidGroupMessageContent
		}
		eventID, err = s.GenerateID("gevtmsg")
		if err != nil {
			return GroupMessageFanoutResult{}, err
		}
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return GroupMessageFanoutResult{}, ErrInvalidGroupMessageContent
	}
	result, err := s.sendGroupMessageFanout(groupID, eventID, content, strings.TrimSpace(threadID))
	if err != nil {
		return GroupMessageFanoutResult{}, err
	}
	s.recordAggregate("send")
	s.logInfo(
		"group message fanout completed",
		"correlation_id", CorrelationID(groupID, eventID),
		"group_id", groupID,
		"event_id", eventID,
		"attempted", result.Attempted,
		"delivered", result.Delivered,
		"pending", result.Pending,
		"failed", result.Failed,
	)
	return result, nil
}

func (s *Service) sendGroupMessageFanout(groupID, eventID, content, threadID string) (GroupMessageFanoutResult, error) {
	fanout := &GroupMessageFanoutService{
		States:             s.SnapshotStates(),
		Abuse:              s.Abuse,
		IdentityID:         s.IdentityID,
		GenerateID:         s.GenerateID,
		ActiveDeviceID:     s.ActiveDeviceID,
		Now:                s.Now,
		IsBlockedSender:    s.IsBlockedSender,
		GetMessage:         s.GetMessage,
		SaveMessage:        s.SaveMessage,
		PrepareAndPublish:  s.PrepareAndPublish,
		RecordError:        s.RecordError,
		NotifyGroupMessage: func(groupID string, msg models.Message) { s.notifyGroupMessage(groupID, msg) },
	}
	return fanout.SendGroupMessageFanout(groupID, eventID, content, threadID)
}

func (s *Service) notifyGroupMessage(groupID string, msg models.Message) {
	if s.Notify == nil {
		return
	}
	s.Notify("notify.group.message.new", map[string]any{
		"group_id": groupID,
		"message":  msg,
	})
}

func (s *Service) ListGroupMessages(groupID string, limit, offset int) ([]models.Message, error) {
	read := &GroupReadService{
		States:                     s.SnapshotStates(),
		ListMessagesByConversation: s.ListMessages,
	}
	return read.ListGroupMessages(groupID, limit, offset)
}

func (s *Service) ListGroupMessagesByThread(groupID, threadID string, limit, offset int) ([]models.Message, error) {
	read := &GroupReadService{
		States:                           s.SnapshotStates(),
		ListMessagesByConversation:       s.ListMessages,
		ListMessagesByConversationThread: s.ListMessagesByThread,
	}
	return read.ListGroupMessagesByThread(groupID, threadID, limit, offset)
}

func (s *Service) GetGroupMessageStatus(groupID, messageID string) (models.MessageStatus, error) {
	read := &GroupReadService{GetMessage: s.GetMessage}
	return read.GetGroupMessageStatus(groupID, messageID)
}

func (s *Service) DeleteGroupMessage(groupID, messageID string) error {
	read := &GroupReadService{
		GetMessage:    s.GetMessage,
		DeleteMessage: s.DeleteMessage,
	}
	if err := read.DeleteGroupMessage(groupID, messageID); err != nil {
		return err
	}
	if s.Notify != nil {
		s.Notify("notify.group.message.deleted", map[string]any{
			"group_id":   groupID,
			"message_id": messageID,
		})
	}
	return nil
}
