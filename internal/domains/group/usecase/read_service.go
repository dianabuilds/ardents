package usecase

import (
	"aim-chat/go-backend/pkg/models"
	"errors"
	"sort"
	"strings"
)

type GroupReadService struct {
	States                           map[string]GroupState
	GetMessage                       func(messageID string) (models.Message, bool)
	DeleteMessage                    func(contactID, messageID string) (bool, error)
	ListMessagesByConversation       func(conversationID, conversationType string, limit, offset int) []models.Message
	ListMessagesByConversationThread func(conversationID, conversationType, threadID string, limit, offset int) []models.Message
}

func (s *GroupReadService) GetGroup(groupID string) (Group, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return Group{}, err
	}
	state, ok := s.States[groupID]
	if !ok {
		return Group{}, ErrGroupNotFound
	}
	return state.Group, nil
}

func (s *GroupReadService) ListGroups() ([]Group, error) {
	out := make([]Group, 0, len(s.States))
	for _, state := range s.States {
		out = append(out, state.Group)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *GroupReadService) ListGroupMembers(groupID string) ([]GroupMember, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return nil, err
	}
	state, ok := s.States[groupID]
	if !ok {
		return nil, ErrGroupNotFound
	}
	out := make([]GroupMember, 0, len(state.Members))
	for _, member := range state.Members {
		out = append(out, member)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].MemberID < out[j].MemberID
	})
	return out, nil
}

func (s *GroupReadService) ListGroupMessages(groupID string, limit, offset int) ([]models.Message, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return nil, err
	}
	if _, ok := s.States[groupID]; !ok {
		return nil, ErrGroupNotFound
	}
	if s.ListMessagesByConversation == nil {
		return nil, errors.New("message list repository is not configured")
	}
	msgs := s.ListMessagesByConversation(groupID, models.ConversationTypeGroup, limit, offset)
	filtered := make([]models.Message, 0, len(msgs))
	for _, msg := range msgs {
		if strings.TrimSpace(msg.ContentType) == groupFanoutTransportContentType {
			continue
		}
		filtered = append(filtered, msg)
	}
	return append([]models.Message(nil), filtered...), nil
}

func (s *GroupReadService) ListGroupMessagesByThread(groupID, threadID string, limit, offset int) ([]models.Message, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return nil, err
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, errors.New("thread id is required")
	}
	if _, ok := s.States[groupID]; !ok {
		return nil, ErrGroupNotFound
	}
	if s.ListMessagesByConversationThread == nil {
		return nil, errors.New("threaded message list repository is not configured")
	}
	msgs := s.ListMessagesByConversationThread(groupID, models.ConversationTypeGroup, threadID, limit, offset)
	filtered := make([]models.Message, 0, len(msgs))
	for _, msg := range msgs {
		if strings.TrimSpace(msg.ContentType) == groupFanoutTransportContentType {
			continue
		}
		filtered = append(filtered, msg)
	}
	return append([]models.Message(nil), filtered...), nil
}

func (s *GroupReadService) GetGroupMessageStatus(groupID, messageID string) (models.MessageStatus, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return models.MessageStatus{}, err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return models.MessageStatus{}, errors.New("message id is required")
	}
	if s.GetMessage == nil {
		return models.MessageStatus{}, errors.New("message repository is not configured")
	}
	msg, ok := s.GetMessage(messageID)
	if !ok {
		return models.MessageStatus{}, errors.New("message not found")
	}
	if msg.ConversationType != models.ConversationTypeGroup || strings.TrimSpace(msg.ConversationID) != groupID {
		return models.MessageStatus{}, errors.New("message does not belong to group")
	}
	return models.MessageStatus{
		MessageID: msg.ID,
		Status:    msg.Status,
	}, nil
}

func (s *GroupReadService) DeleteGroupMessage(groupID, messageID string) error {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return errors.New("message id is required")
	}
	if s.GetMessage == nil || s.DeleteMessage == nil {
		return errors.New("message repository is not configured")
	}
	msg, ok := s.GetMessage(messageID)
	if !ok {
		return errors.New("message not found")
	}
	if msg.ConversationType != models.ConversationTypeGroup || strings.TrimSpace(msg.ConversationID) != groupID {
		return errors.New("message does not belong to group")
	}
	deleted, err := s.DeleteMessage(msg.ContactID, messageID)
	if err != nil {
		return err
	}
	if !deleted {
		return errors.New("message not found")
	}
	return nil
}
