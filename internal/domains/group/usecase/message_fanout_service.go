package usecase

import (
	"aim-chat/go-backend/pkg/models"
	"math/rand"
	"strings"
	"time"
)

type GroupMessageWireMeta struct {
	GroupID           string
	EventID           string
	MembershipVersion uint64
	GroupKeyVersion   uint32
	SenderDeviceID    string
}

type GroupMessageFanoutService struct {
	States map[string]GroupState
	Abuse  *AbuseProtection

	IdentityID         func() string
	GenerateID         func(prefix string) (string, error)
	ActiveDeviceID     func() (string, error)
	Now                func() time.Time
	IsBlockedSender    func(string) bool
	GetMessage         func(string) (models.Message, bool)
	SaveMessage        func(models.Message) error
	PrepareAndPublish  func(msg models.Message, recipientID string, meta GroupMessageWireMeta) (sentID string, category string, err error)
	RecordError        func(category string, err error)
	NotifyGroupMessage func(groupID string, msg models.Message)
}

const groupFanoutTransportContentType = "group_fanout_transport"

type fanoutContext struct {
	groupID         string
	eventID         string
	content         string
	threadID        string
	actorID         string
	deviceID        string
	now             time.Time
	state           GroupState
	groupKeyVersion uint32
}

func (s *GroupMessageFanoutService) SendGroupMessageFanout(groupID, eventID, content, threadID string) (GroupMessageFanoutResult, error) {
	ctx, err := s.prepareFanoutContext(groupID, eventID, content, threadID)
	if err != nil {
		return GroupMessageFanoutResult{}, err
	}
	recipients := s.collectRecipients(ctx.state, ctx.actorID, ctx.now)
	result := GroupMessageFanoutResult{
		GroupID:    ctx.groupID,
		EventID:    ctx.eventID,
		Attempted:  len(recipients),
		Recipients: make([]GroupMessageRecipientStatus, 0, len(recipients)),
	}
	s.persistSenderMessage(ctx)
	for _, recipientID := range recipients {
		if err := s.processRecipient(ctx, recipientID, &result); err != nil {
			return GroupMessageFanoutResult{}, err
		}
	}
	return result, nil
}

func (s *GroupMessageFanoutService) prepareFanoutContext(groupID, eventID, content, threadID string) (fanoutContext, error) {
	normalizedGroupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return fanoutContext{}, err
	}
	normalizedEventID, err := s.resolveEventID(eventID)
	if err != nil {
		return fanoutContext{}, err
	}
	normalizedContent := strings.TrimSpace(content)
	if normalizedContent == "" {
		return fanoutContext{}, ErrInvalidGroupMessageContent
	}
	actorID, err := s.resolveActorID()
	if err != nil {
		return fanoutContext{}, err
	}
	now := s.resolveNow()
	if s.Abuse != nil && !s.Abuse.AllowSend(actorID, now) {
		return fanoutContext{}, ErrGroupRateLimitExceeded
	}
	if s.ActiveDeviceID == nil {
		return fanoutContext{}, ErrInvalidGroupEventPayload
	}
	deviceID, err := s.ActiveDeviceID()
	if err != nil {
		return fanoutContext{}, err
	}
	state, ok := s.States[normalizedGroupID]
	if !ok {
		return fanoutContext{}, ErrGroupNotFound
	}
	if err := validateActorCanFanout(state, actorID); err != nil {
		return fanoutContext{}, err
	}
	groupKeyVersion := state.LastKeyVersion
	if groupKeyVersion == 0 {
		groupKeyVersion = 1
	}
	return fanoutContext{
		groupID:         normalizedGroupID,
		eventID:         normalizedEventID,
		content:         normalizedContent,
		threadID:        strings.TrimSpace(threadID),
		actorID:         actorID,
		deviceID:        deviceID,
		now:             now,
		state:           state,
		groupKeyVersion: groupKeyVersion,
	}, nil
}

func (s *GroupMessageFanoutService) resolveEventID(eventID string) (string, error) {
	trimmedEventID := strings.TrimSpace(eventID)
	if trimmedEventID != "" {
		return trimmedEventID, nil
	}
	if s.GenerateID == nil {
		return "", ErrInvalidGroupMessageContent
	}
	return s.GenerateID("gevtmsg")
}

func (s *GroupMessageFanoutService) resolveActorID() (string, error) {
	if s.IdentityID == nil {
		return "", ErrInvalidGroupMemberID
	}
	actorID := strings.TrimSpace(s.IdentityID())
	if actorID == "" {
		return "", ErrInvalidGroupMemberID
	}
	return actorID, nil
}

func (s *GroupMessageFanoutService) resolveNow() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func validateActorCanFanout(state GroupState, actorID string) error {
	actor, ok := state.Members[actorID]
	if !ok || actor.Status != GroupMemberStatusActive {
		return ErrGroupPermissionDenied
	}
	if isChannelGroupTitle(state.Group.Title) && actor.Role != GroupMemberRoleOwner && actor.Role != GroupMemberRoleAdmin {
		return ErrGroupPermissionDenied
	}
	return nil
}

func (s *GroupMessageFanoutService) collectRecipients(state GroupState, actorID string, now time.Time) []string {
	recipients := make([]string, 0, len(state.Members))
	for memberID, member := range state.Members {
		if memberID == actorID || member.Status != GroupMemberStatusActive {
			continue
		}
		if s.IsBlockedSender != nil && s.IsBlockedSender(memberID) {
			continue
		}
		recipients = append(recipients, memberID)
	}
	if len(recipients) <= 1 {
		return recipients
	}
	r := rand.New(rand.NewSource(now.UnixNano()))
	r.Shuffle(len(recipients), func(i, j int) { recipients[i], recipients[j] = recipients[j], recipients[i] })
	return recipients
}

func (s *GroupMessageFanoutService) persistSenderMessage(ctx fanoutContext) {
	if s.SaveMessage == nil {
		return
	}
	senderMessageID := DeriveRecipientMessageID(ctx.eventID, ctx.actorID)
	if s.GetMessage != nil {
		if existing, exists := s.GetMessage(senderMessageID); exists {
			if s.NotifyGroupMessage != nil {
				s.NotifyGroupMessage(ctx.groupID, existing)
			}
			return
		}
	}
	senderMsg := models.Message{
		ID:               senderMessageID,
		ContactID:        ctx.actorID,
		ConversationID:   ctx.groupID,
		ConversationType: models.ConversationTypeGroup,
		ThreadID:         ctx.threadID,
		Content:          []byte(ctx.content),
		Timestamp:        ctx.now,
		Direction:        "out",
		Status:           "sent",
		ContentType:      "text",
	}
	if err := s.SaveMessage(senderMsg); err != nil {
		if s.RecordError != nil {
			s.RecordError("storage", err)
		}
		return
	}
	if s.NotifyGroupMessage != nil {
		s.NotifyGroupMessage(ctx.groupID, senderMsg)
	}
}

func (s *GroupMessageFanoutService) processRecipient(ctx fanoutContext, recipientID string, result *GroupMessageFanoutResult) error {
	messageID := DeriveRecipientMessageID(ctx.eventID, recipientID)
	if s.GetMessage != nil {
		if existing, exists := s.GetMessage(messageID); exists {
			result.Recipients = append(result.Recipients, GroupMessageRecipientStatus{
				RecipientID: recipientID,
				MessageID:   messageID,
				Status:      existing.Status,
				Duplicate:   true,
			})
			if existing.Status == "pending" {
				result.Pending++
			} else {
				result.Delivered++
			}
			return nil
		}
	}
	if s.SaveMessage == nil {
		return ErrGroupNotFound
	}
	msg := models.Message{
		ID:               messageID,
		ContactID:        recipientID,
		ConversationID:   ctx.groupID,
		ConversationType: models.ConversationTypeGroup,
		ThreadID:         ctx.threadID,
		Content:          []byte(ctx.content),
		Timestamp:        ctx.now,
		Direction:        "out",
		Status:           "pending",
		ContentType:      groupFanoutTransportContentType,
	}
	if err := s.SaveMessage(msg); err != nil {
		if s.RecordError != nil {
			s.RecordError("storage", err)
		}
		s.appendRecipientFailure(result, recipientID, messageID, err)
		return nil
	}
	if s.PrepareAndPublish == nil {
		return ErrGroupNotFound
	}
	sentID, category, err := s.PrepareAndPublish(msg, recipientID, GroupMessageWireMeta{
		GroupID:           ctx.groupID,
		EventID:           ctx.eventID,
		MembershipVersion: ctx.state.Version,
		GroupKeyVersion:   ctx.groupKeyVersion,
		SenderDeviceID:    ctx.deviceID,
	})
	if err != nil {
		if category != "" && s.RecordError != nil {
			s.RecordError(category, err)
		}
		s.appendRecipientFailure(result, recipientID, messageID, err)
		return nil
	}
	statusValue := "sent"
	if s.GetMessage != nil {
		if saved, ok := s.GetMessage(sentID); ok {
			statusValue = saved.Status
		}
	}
	if statusValue == "pending" {
		result.Pending++
	} else {
		result.Delivered++
	}
	result.Recipients = append(result.Recipients, GroupMessageRecipientStatus{
		RecipientID: recipientID,
		MessageID:   messageID,
		Status:      statusValue,
	})
	return nil
}

func (s *GroupMessageFanoutService) appendRecipientFailure(
	result *GroupMessageFanoutResult,
	recipientID string,
	messageID string,
	err error,
) {
	result.Failed++
	result.Recipients = append(result.Recipients, GroupMessageRecipientStatus{
		RecipientID: recipientID,
		MessageID:   messageID,
		Status:      "failed",
		Error:       err.Error(),
	})
}

func isChannelGroupTitle(title string) bool {
	normalized := strings.ToLower(strings.TrimSpace(title))
	return strings.HasPrefix(normalized, "[channel")
}
