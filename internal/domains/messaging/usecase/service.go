package usecase

import (
	"aim-chat/go-backend/internal/domains/contracts"
	messagingpolicy "aim-chat/go-backend/internal/domains/messaging/policy"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
	"errors"
	"strings"
	"time"
)

type ServiceDeps struct {
	Identity contracts.IdentityDomain
	Sessions contracts.SessionDomain
	Messages contracts.MessageRepository

	GenerateID          func(prefix string) (string, error)
	TrackOperation      func(operation string, errRef *error) func()
	PublishQueued       func(msg models.Message, contactID string, wire contracts.WirePayload) (string, error)
	ApplyAutoRead       func(message *models.Message, contactID string)
	PublishPrivate      func(msg waku.PrivateMessage) error
	Notify              func(method string, payload any)
	RecordError         func(category string, err error)
	IsMessageIDConflict func(err error) bool
}

type Service struct {
	deps ServiceDeps
}

func NewService(deps ServiceDeps) *Service {
	return &Service{deps: deps}
}

func (s *Service) SendMessage(contactID, content string) (msgID string, err error) {
	return s.sendMessageWithThread(contactID, content, "")
}

func (s *Service) SendMessageInThread(contactID, content, threadID string) (msgID string, err error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return "", errors.New("thread id is required")
	}
	return s.sendMessageWithThread(contactID, content, threadID)
}

func (s *Service) sendMessageWithThread(contactID, content, threadID string) (msgID string, err error) {
	if s.deps.TrackOperation != nil {
		defer s.deps.TrackOperation("message.send", &err)()
	}
	contactID, content, err = ParseSendMessageInput(contactID, content)
	if err != nil {
		return "", err
	}
	if !s.deps.Identity.HasContact(contactID) {
		return "", errors.New("contact is not added")
	}

	draft := BuildOutboundDraft("draft", contactID, content, time.Now())
	draft.ThreadID = threadID
	wire, werr := s.BuildStoredMessageWire(draft)
	if werr != nil {
		s.deps.RecordError(contracts.ErrorCategoryCrypto, werr)
		return "", werr
	}

	msg, err := AllocateOutboundMessage(
		contactID,
		content,
		threadID,
		time.Now,
		func() (string, error) { return s.deps.GenerateID("msg") },
		func(msg models.Message) error {
			err := s.deps.Messages.SaveMessage(msg)
			if err != nil && (s.deps.IsMessageIDConflict == nil || !s.deps.IsMessageIDConflict(err)) {
				s.deps.RecordError(contracts.ErrorCategoryStorage, err)
			}
			return err
		},
		s.deps.IsMessageIDConflict,
	)
	if err != nil {
		return "", err
	}

	s.deps.Notify("notify.message.new", map[string]any{
		"contact_id": contactID,
		"message":    msg,
	})
	return s.deps.PublishQueued(msg, contactID, wire)
}

func (s *Service) EditMessage(contactID, messageID, content string) (models.Message, error) {
	contactID, messageID, content, err := ParseEditMessageInput(contactID, messageID, content)
	if err != nil {
		return models.Message{}, err
	}
	msg, ok := s.deps.Messages.GetMessage(messageID)
	if err := ValidateEditableMessage(msg, ok, contactID); err != nil {
		return models.Message{}, err
	}

	updated, ok, err := s.deps.Messages.UpdateMessageContent(messageID, []byte(content), msg.ContentType)
	if err != nil {
		s.deps.RecordError(contracts.ErrorCategoryStorage, err)
		return models.Message{}, err
	}
	if !ok {
		return models.Message{}, errors.New("message not found")
	}

	s.deps.Notify("notify.message.status", map[string]any{
		"contact_id": contactID,
		"message_id": messageID,
		"status":     updated.Status,
		"edited":     true,
	})
	return updated, nil
}

func (s *Service) DeleteMessage(contactID, messageID string) error {
	contactID = strings.TrimSpace(contactID)
	messageID = strings.TrimSpace(messageID)
	if contactID == "" || messageID == "" {
		return errors.New("invalid params")
	}
	deleted, err := s.deps.Messages.DeleteMessage(contactID, messageID)
	if err != nil {
		s.deps.RecordError(contracts.ErrorCategoryStorage, err)
		return err
	}
	if !deleted {
		return errors.New("message not found")
	}
	s.deps.Notify("notify.message.deleted", map[string]any{
		"contact_id": contactID,
		"message_id": messageID,
	})
	return nil
}

func (s *Service) ClearMessages(contactID string) (int, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return 0, errors.New("invalid params")
	}
	deleted, err := s.deps.Messages.ClearMessages(contactID)
	if err != nil {
		s.deps.RecordError(contracts.ErrorCategoryStorage, err)
		return 0, err
	}
	s.deps.Notify("notify.message.cleared", map[string]any{
		"contact_id": contactID,
		"count":      deleted,
	})
	return deleted, nil
}

func (s *Service) GetMessages(contactID string, limit, offset int) (messages []models.Message, err error) {
	if s.deps.TrackOperation != nil {
		defer s.deps.TrackOperation("message.list", &err)()
	}
	contactID, err = ParseMessageListContactID(contactID)
	if err != nil {
		return nil, err
	}
	messages = s.deps.Messages.ListMessages(contactID, limit, offset)
	for i := range messages {
		msg := messages[i]
		if !ShouldAutoReadOnList(msg) {
			continue
		}
		s.deps.ApplyAutoRead(&messages[i], contactID)
	}
	return messages, nil
}

func (s *Service) GetMessagesByThread(contactID, threadID string, limit, offset int) (messages []models.Message, err error) {
	if s.deps.TrackOperation != nil {
		defer s.deps.TrackOperation("message.list", &err)()
	}
	contactID, err = ParseMessageListContactID(contactID)
	if err != nil {
		return nil, err
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, errors.New("thread id is required")
	}
	messages = s.deps.Messages.ListMessagesByConversationThread(contactID, models.ConversationTypeDirect, threadID, limit, offset)
	for i := range messages {
		msg := messages[i]
		if !ShouldAutoReadOnList(msg) {
			continue
		}
		s.deps.ApplyAutoRead(&messages[i], contactID)
	}
	return messages, nil
}

func (s *Service) GetMessageStatus(messageID string) (status models.MessageStatus, err error) {
	if s.deps.TrackOperation != nil {
		defer s.deps.TrackOperation("message.status", &err)()
	}
	messageID, err = ParseMessageStatusID(messageID)
	if err != nil {
		return models.MessageStatus{}, err
	}
	msg, ok := s.deps.Messages.GetMessage(messageID)
	return ComposeMessageStatus(msg, ok)
}

func (s *Service) BuildStoredMessageWire(msg models.Message) (contracts.WirePayload, error) {
	wire, _, err := BuildWireForOutboundMessage(msg, s.deps.Sessions)
	if errors.Is(err, messagingpolicy.ErrOutboundSessionRequired) {
		card, cardErr := s.deps.Identity.SelfContactCard(s.deps.Identity.GetIdentity().ID)
		if cardErr != nil {
			return contracts.WirePayload{}, contracts.WrapCategorizedError(contracts.ErrorCategoryCrypto, err)
		}
		plainWire := NewPlainWire(msg.Content)
		plainWire.ThreadID = strings.TrimSpace(msg.ThreadID)
		plainWire.Card = &card
		return plainWire, nil
	}
	if err != nil {
		return contracts.WirePayload{}, contracts.WrapCategorizedError(contracts.ErrorCategoryCrypto, err)
	}
	wire.ThreadID = strings.TrimSpace(msg.ThreadID)
	return wire, nil
}

func (s *Service) InitSession(contactID string, peerPublicKey []byte) (session models.SessionState, err error) {
	if s.deps.TrackOperation != nil {
		defer s.deps.TrackOperation("session.init", &err)()
	}
	contactID = NormalizeSessionContact(contactID)
	if err := RequireVerifiedContact(s.deps.Identity.HasVerifiedContact(contactID)); err != nil {
		return models.SessionState{}, err
	}
	localIdentity := s.deps.Identity.GetIdentity()
	state, err := s.deps.Sessions.InitSession(localIdentity.ID, contactID, peerPublicKey)
	if err != nil {
		s.deps.RecordError(contracts.ErrorCategoryCrypto, err)
		return models.SessionState{}, err
	}
	return MapSessionState(state), nil
}

func (s *Service) RevokeDevice(deviceID string) (models.DeviceRevocation, error) {
	deviceID = NormalizeDeviceIDForRevocation(deviceID)
	rev, err := s.deps.Identity.RevokeDevice(deviceID)
	if err != nil {
		return models.DeviceRevocation{}, err
	}
	contacts := s.deps.Identity.Contacts()
	payloadBytes, err := BuildDeviceRevocationPayload(rev)
	if err != nil {
		s.deps.RecordError(contracts.ErrorCategoryAPI, err)
		return models.DeviceRevocation{}, err
	}
	localIdentity := s.deps.Identity.GetIdentity()
	failures := DispatchDeviceRevocation(localIdentity.ID, contacts, payloadBytes, func() (string, error) {
		return s.deps.GenerateID("rev")
	}, s.deps.PublishPrivate)
	for _, f := range failures {
		if f.Err != nil {
			s.deps.RecordError(f.Category, f.Err)
		}
	}
	if deliveryErr := BuildDeviceRevocationDeliveryError(len(contacts), failures); deliveryErr != nil {
		return rev, deliveryErr
	}
	return rev, nil
}
