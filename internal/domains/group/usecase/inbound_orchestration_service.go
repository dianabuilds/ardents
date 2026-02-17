package usecase

import (
	"aim-chat/go-backend/pkg/models"
	"errors"
	"strings"
	"time"
)

type InboundGroupMessageParams struct {
	MessageID         string
	SenderID          string
	Payload           []byte
	ConversationID    string
	EventID           string
	MembershipVersion uint64
	GroupKeyVersion   uint32
	SenderDeviceID    string
}

type InboundGroupEventParams struct {
	SenderID          string
	RecipientID       string
	ConversationID    string
	EventID           string
	EventType         string
	MembershipVersion uint64
	SenderDeviceID    string
	Plain             []byte
	HasDevice         bool
	DeviceID          string
}

type InboundOrchestrationService struct {
	States   map[string]GroupState
	EventLog map[string][]GroupEvent
	Persist  SnapshotPersist

	Now                   func() time.Time
	IdentityID            func() string
	IsBlockedSender       func(string) bool
	GuardReplay           func(kind, groupID, senderDeviceID, uniqueID string, occurredAt, now time.Time) error
	ResolveInboundContent func() ([]byte, string, error)
	BuildStoredMessage    func(content []byte, contentType string, now time.Time) models.Message
	SaveMessage           func(models.Message) error
	GetMessage            func(messageID string) (models.Message, bool)
	IsMessageIDConflict   func(error) bool
	NotifyGroupMessage    func(groupID string, msg models.Message)
	NotifyGroupUpdated    func(event GroupEvent)

	RecordError          func(category string, err error)
	RecordGroupAggregate func(name string)
	Warn                 func(message string, args ...any)
	Debug                func(message string, args ...any)
}

func (s *InboundOrchestrationService) HandleInboundGroupMessage(in InboundGroupMessageParams) {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	correlationID := CorrelationID(in.ConversationID, in.EventID)
	if s.IsBlockedSender != nil && s.IsBlockedSender(in.SenderID) {
		s.recordErr("crypto", ErrGroupSenderBlocked)
		s.recordAggregate("policy_reject")
		s.warn("group message rejected", "reason", "blocked_sender", "correlation_id", correlationID, "group_id", in.ConversationID, "event_id", in.EventID, "actor_id", in.SenderID)
		return
	}
	state, ok := s.States[strings.TrimSpace(in.ConversationID)]
	if !ok {
		s.recordErr("crypto", ErrGroupNotFound)
		s.recordAggregate("policy_reject")
		s.warn("group message rejected", "reason", "unknown_group", "correlation_id", correlationID, "group_id", in.ConversationID, "event_id", in.EventID, "actor_id", in.SenderID)
		return
	}
	reason, err := ValidateInboundGroupMessageState(
		state,
		strings.TrimSpace(in.SenderID),
		in.MembershipVersion,
		in.GroupKeyVersion,
	)
	if err != nil {
		s.recordErr("crypto", err)
		s.recordAggregate("policy_reject")
		logArgs := []any{
			"reason", reason,
			"correlation_id", correlationID,
			"group_id", in.ConversationID,
			"event_id", in.EventID,
			"actor_id", in.SenderID,
		}
		if reason == InboundGroupMessageReasonMembershipVersionMismatch {
			logArgs = append(logArgs, "wire_version", in.MembershipVersion, "state_version", state.Version)
		}
		if reason == InboundGroupMessageReasonGroupKeyVersionMismatch {
			expectedGroupKeyVersion := state.LastKeyVersion
			if expectedGroupKeyVersion == 0 {
				expectedGroupKeyVersion = 1
			}
			logArgs = append(logArgs, "wire_key_version", in.GroupKeyVersion, "state_key_version", expectedGroupKeyVersion)
		}
		s.warn("group message rejected", logArgs...)
		return
	}
	replayID := strings.TrimSpace(in.EventID)
	if replayID == "" {
		replayID = strings.TrimSpace(in.MessageID)
	}
	if s.GuardReplay != nil {
		if err := s.GuardReplay("message", in.ConversationID, in.SenderDeviceID, replayID, now, now); err != nil {
			s.recordErr("crypto", err)
			s.recordAggregate("policy_reject")
			s.warn("group message rejected", "reason", "replay_guard", "correlation_id", correlationID, "group_id", in.ConversationID, "event_id", in.EventID, "actor_id", in.SenderID, "error", err.Error())
			return
		}
	}

	content := append([]byte(nil), in.Payload...)
	contentType := "text"
	if s.ResolveInboundContent != nil {
		decrypted, resolvedType, decryptErr := s.ResolveInboundContent()
		if decryptErr != nil {
			s.recordErr("crypto", decryptErr)
			s.recordAggregate("decrypt_fail")
			s.warn("group message decrypt failed", "correlation_id", correlationID, "group_id", in.ConversationID, "event_id", in.EventID, "actor_id", in.SenderID, "error", decryptErr.Error())
		} else {
			content = decrypted
			contentType = resolvedType
		}
	}

	if s.BuildStoredMessage == nil || s.SaveMessage == nil {
		return
	}
	stored := s.BuildStoredMessage(content, contentType, now)
	if err := s.SaveMessage(stored); err != nil {
		if s.IsMessageIDConflict != nil && s.IsMessageIDConflict(err) {
			s.warn("inbound group message id conflict ignored", "message_id", stored.ID, "group_id", stored.ConversationID)
			return
		}
		s.recordErr("storage", err)
		return
	}
	if s.NotifyGroupMessage != nil {
		s.NotifyGroupMessage(in.ConversationID, stored)
	}
}

func (s *InboundOrchestrationService) HandleInboundGroupEvent(in InboundGroupEventParams) {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	correlationID := CorrelationID(in.ConversationID, in.EventID)
	if s.IsBlockedSender != nil && s.IsBlockedSender(in.SenderID) {
		s.recordErr("crypto", ErrGroupSenderBlocked)
		s.recordAggregate("policy_reject")
		s.warn("group event rejected", "reason", "blocked_sender", "correlation_id", correlationID, "group_id", in.ConversationID, "event_id", in.EventID, "actor_id", in.SenderID)
		return
	}
	event, err := DecodeInboundGroupEvent(InboundGroupEventWire{
		EventID:           in.EventID,
		ConversationID:    in.ConversationID,
		MembershipVersion: in.MembershipVersion,
		EventType:         in.EventType,
		Plain:             in.Plain,
		SenderID:          in.SenderID,
		RecipientID:       in.RecipientID,
	}, now)
	if err != nil {
		s.recordErr("api", err)
		s.recordAggregate("policy_reject")
		s.warn("group event rejected", "reason", "invalid_payload", "correlation_id", correlationID, "group_id", in.ConversationID, "event_id", in.EventID, "actor_id", in.SenderID)
		return
	}
	if in.HasDevice && strings.TrimSpace(in.SenderDeviceID) != strings.TrimSpace(in.DeviceID) {
		s.recordErr("crypto", errors.New("sender device mismatch"))
		s.recordAggregate("policy_reject")
		s.warn("group event rejected", "reason", "sender_device_mismatch", "correlation_id", correlationID, "group_id", in.ConversationID, "event_id", in.EventID, "actor_id", in.SenderID)
		return
	}
	if s.GuardReplay != nil {
		if err := s.GuardReplay("event", event.GroupID, in.SenderDeviceID, event.ID, event.OccurredAt, now); err != nil {
			s.recordErr("crypto", err)
			s.recordAggregate("policy_reject")
			s.warn("group event rejected", "reason", "replay_guard", "correlation_id", correlationID, "group_id", event.GroupID, "event_id", event.ID, "actor_id", event.ActorID, "error", err.Error())
			return
		}
	}

	if s.IdentityID == nil {
		return
	}
	state, err := EnsureInboundEventState(s.States, event, s.IdentityID())
	if err != nil {
		s.recordErr("crypto", err)
		s.recordAggregate("policy_reject")
		s.warn("group event rejected", "reason", "unknown_group", "correlation_id", correlationID, "group_id", event.GroupID, "event_id", event.ID, "actor_id", event.ActorID)
		return
	}
	if err := AuthorizeInboundGroupEvent(state, event); err != nil {
		s.recordErr("crypto", err)
		s.recordAggregate("policy_reject")
		s.warn("group event rejected", "reason", "unauthorized", "correlation_id", correlationID, "group_id", event.GroupID, "event_id", event.ID, "actor_id", event.ActorID, "event_type", event.Type)
		return
	}

	_, applied, err := ApplyEventsWithRollback(state, s.States, s.EventLog, s.Persist, event)
	if err != nil {
		s.recordErr("storage", err)
		s.recordAggregate("policy_reject")
		s.warn("group event rejected", "reason", "persist_failed", "correlation_id", correlationID, "group_id", event.GroupID, "event_id", event.ID, "actor_id", event.ActorID)
		return
	}
	if len(applied) == 0 {
		s.debug("group event duplicate ignored", "group_id", event.GroupID, "event_id", event.ID, "actor_id", event.ActorID)
		return
	}
	if s.NotifyGroupUpdated != nil {
		s.NotifyGroupUpdated(event)
	}
}

func (s *InboundOrchestrationService) recordErr(category string, err error) {
	if s.RecordError != nil && err != nil {
		s.RecordError(category, err)
	}
}

func (s *InboundOrchestrationService) recordAggregate(name string) {
	if s.RecordGroupAggregate != nil {
		s.RecordGroupAggregate(name)
	}
}

func (s *InboundOrchestrationService) warn(message string, args ...any) {
	if s.Warn != nil {
		s.Warn(message, args...)
	}
}

func (s *InboundOrchestrationService) debug(message string, args ...any) {
	if s.Debug != nil {
		s.Debug(message, args...)
	}
}
