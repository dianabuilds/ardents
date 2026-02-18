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

func (s *GroupMessageFanoutService) SendGroupMessageFanout(groupID, eventID, content, threadID string) (GroupMessageFanoutResult, error) {
	groupID, err := NormalizeGroupID(groupID)
	if err != nil {
		return GroupMessageFanoutResult{}, err
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		if s.GenerateID == nil {
			return GroupMessageFanoutResult{}, ErrInvalidGroupMessageContent
		}
		generated, genErr := s.GenerateID("gevtmsg")
		if genErr != nil {
			return GroupMessageFanoutResult{}, genErr
		}
		eventID = generated
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return GroupMessageFanoutResult{}, ErrInvalidGroupMessageContent
	}
	threadID = strings.TrimSpace(threadID)
	if s.IdentityID == nil {
		return GroupMessageFanoutResult{}, ErrInvalidGroupMemberID
	}
	actorID := strings.TrimSpace(s.IdentityID())
	if actorID == "" {
		return GroupMessageFanoutResult{}, ErrInvalidGroupMemberID
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	if s.Abuse != nil && !s.Abuse.AllowSend(actorID, now) {
		return GroupMessageFanoutResult{}, ErrGroupRateLimitExceeded
	}
	if s.ActiveDeviceID == nil {
		return GroupMessageFanoutResult{}, ErrInvalidGroupEventPayload
	}
	deviceID, err := s.ActiveDeviceID()
	if err != nil {
		return GroupMessageFanoutResult{}, err
	}

	state, ok := s.States[groupID]
	if !ok {
		return GroupMessageFanoutResult{}, ErrGroupNotFound
	}
	actor, ok := state.Members[actorID]
	if !ok || actor.Status != GroupMemberStatusActive {
		return GroupMessageFanoutResult{}, ErrGroupPermissionDenied
	}
	if isChannelGroupTitle(state.Group.Title) && actor.Role != GroupMemberRoleOwner && actor.Role != GroupMemberRoleAdmin {
		return GroupMessageFanoutResult{}, ErrGroupPermissionDenied
	}
	groupKeyVersion := state.LastKeyVersion
	if groupKeyVersion == 0 {
		groupKeyVersion = 1
	}

	recipients := make([]string, 0, len(state.Members))
	for memberID, member := range state.Members {
		if memberID == actorID {
			continue
		}
		if member.Status != GroupMemberStatusActive {
			continue
		}
		if s.IsBlockedSender != nil && s.IsBlockedSender(memberID) {
			continue
		}
		recipients = append(recipients, memberID)
	}
	if len(recipients) > 1 {
		r := rand.New(rand.NewSource(now.UnixNano()))
		r.Shuffle(len(recipients), func(i, j int) {
			recipients[i], recipients[j] = recipients[j], recipients[i]
		})
	}

	result := GroupMessageFanoutResult{
		GroupID:    groupID,
		EventID:    eventID,
		Attempted:  len(recipients),
		Recipients: make([]GroupMessageRecipientStatus, 0, len(recipients)),
	}

	// Persist a canonical sender-visible message so group history includes own sends
	// even when there are no other active members in the group.
	if s.SaveMessage != nil {
		senderMessageID := DeriveRecipientMessageID(eventID, actorID)
		senderStored := false
		if s.GetMessage != nil {
			if existing, exists := s.GetMessage(senderMessageID); exists {
				senderStored = true
				if s.NotifyGroupMessage != nil {
					s.NotifyGroupMessage(groupID, existing)
				}
			}
		}
		if !senderStored {
			senderMsg := models.Message{
				ID:               senderMessageID,
				ContactID:        actorID,
				ConversationID:   groupID,
				ConversationType: models.ConversationTypeGroup,
				ThreadID:         threadID,
				Content:          []byte(content),
				Timestamp:        now,
				Direction:        "out",
				Status:           "sent",
				ContentType:      "text",
			}
			if err := s.SaveMessage(senderMsg); err != nil {
				if s.RecordError != nil {
					s.RecordError("storage", err)
				}
			} else if s.NotifyGroupMessage != nil {
				s.NotifyGroupMessage(groupID, senderMsg)
			}
		}
	}

	for _, recipientID := range recipients {
		messageID := DeriveRecipientMessageID(eventID, recipientID)
		if s.GetMessage != nil {
			if existing, exists := s.GetMessage(messageID); exists {
				status := GroupMessageRecipientStatus{
					RecipientID: recipientID,
					MessageID:   messageID,
					Status:      existing.Status,
					Duplicate:   true,
				}
				result.Recipients = append(result.Recipients, status)
				if existing.Status == "pending" {
					result.Pending++
				} else {
					result.Delivered++
				}
				continue
			}
		}

		msg := models.Message{
			ID:               messageID,
			ContactID:        recipientID,
			ConversationID:   groupID,
			ConversationType: models.ConversationTypeGroup,
			ThreadID:         threadID,
			Content:          []byte(content),
			Timestamp:        now,
			Direction:        "out",
			Status:           "pending",
			ContentType:      groupFanoutTransportContentType,
		}
		if s.SaveMessage == nil {
			return GroupMessageFanoutResult{}, ErrGroupNotFound
		}
		if err := s.SaveMessage(msg); err != nil {
			if s.RecordError != nil {
				s.RecordError("storage", err)
			}
			result.Failed++
			result.Recipients = append(result.Recipients, GroupMessageRecipientStatus{
				RecipientID: recipientID,
				MessageID:   messageID,
				Status:      "failed",
				Error:       err.Error(),
			})
			continue
		}

		if s.PrepareAndPublish == nil {
			return GroupMessageFanoutResult{}, ErrGroupNotFound
		}
		sentID, category, err := s.PrepareAndPublish(msg, recipientID, GroupMessageWireMeta{
			GroupID:           groupID,
			EventID:           eventID,
			MembershipVersion: state.Version,
			GroupKeyVersion:   groupKeyVersion,
			SenderDeviceID:    deviceID,
		})
		if err != nil {
			if category != "" && s.RecordError != nil {
				s.RecordError(category, err)
			}
			result.Failed++
			result.Recipients = append(result.Recipients, GroupMessageRecipientStatus{
				RecipientID: recipientID,
				MessageID:   messageID,
				Status:      "failed",
				Error:       err.Error(),
			})
			continue
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
	}
	return result, nil
}

func isChannelGroupTitle(title string) bool {
	normalized := strings.ToLower(strings.TrimSpace(title))
	return strings.HasPrefix(normalized, "[channel")
}
