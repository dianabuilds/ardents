package daemonservice

import (
	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	inboxapp "aim-chat/go-backend/internal/domains/inbox"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
	"errors"
	"strings"
	"time"
)

func (s *Service) handleIncomingPrivateMessage(msg waku.PrivateMessage) {
	s.inboundMessagingCore.HandleIncomingPrivateMessage(toInboundPrivateMessage(msg))
}

func (s *Service) handleInboundGroupMessage(msg messagingapp.InboundPrivateMessage, wire contracts.WirePayload) {
	s.groupRuntime.StateMu.RLock()
	state, ok := s.groupRuntime.States[strings.TrimSpace(wire.ConversationID)]
	s.groupRuntime.StateMu.RUnlock()
	states := map[string]groupdomain.GroupState{}
	if ok {
		states[strings.TrimSpace(wire.ConversationID)] = state
	}
	svc := &groupdomain.InboundOrchestrationService{
		States:          states,
		Now:             time.Now,
		IsBlockedSender: s.privacyCore.IsBlockedSender,
		GuardReplay:     s.guardInboundGroupReplay,
		ResolveInboundContent: func() ([]byte, string, error) {
			return messagingapp.ResolveInboundContent(msg, wire, s.sessionManager)
		},
		BuildStoredMessage: func(content []byte, contentType string, now time.Time) models.Message {
			return messagingapp.BuildInboundGroupStoredMessage(msg, wire.ConversationID, wire.ThreadID, content, contentType, now)
		},
		SaveMessage:         s.messageStore.SaveMessage,
		IsMessageIDConflict: func(err error) bool { return errors.Is(err, storage.ErrMessageIDConflict) },
		NotifyGroupMessage: func(groupID string, stored models.Message) {
			s.notify("notify.group.message.new", map[string]any{
				"group_id": groupID,
				"message":  stored,
			})
		},
		RecordError:          s.recordError,
		RecordGroupAggregate: s.recordGroupAggregate,
		Warn:                 s.logger.Warn,
		Debug:                s.logger.Debug,
	}
	svc.HandleInboundGroupMessage(groupdomain.InboundGroupMessageParams{
		MessageID:         msg.ID,
		SenderID:          msg.SenderID,
		Payload:           msg.Payload,
		ConversationID:    wire.ConversationID,
		EventID:           wire.EventID,
		MembershipVersion: wire.MembershipVersion,
		GroupKeyVersion:   wire.GroupKeyVersion,
		SenderDeviceID:    wire.SenderDeviceID,
	})
}

func (s *Service) handleInboundGroupEvent(msg messagingapp.InboundPrivateMessage, wire contracts.WirePayload) {
	now := time.Now().UTC()
	event, err := groupdomain.DecodeInboundGroupEvent(groupdomain.InboundGroupEventWire{
		EventID:           wire.EventID,
		ConversationID:    wire.ConversationID,
		MembershipVersion: wire.MembershipVersion,
		EventType:         wire.EventType,
		Plain:             wire.Plain,
		SenderID:          msg.SenderID,
		RecipientID:       msg.Recipient,
	}, now)
	if err != nil {
		s.recordError(contracts.ErrorCategoryAPI, err)
		s.recordGroupAggregate("policy_reject")
		s.logger.Warn(
			"group event rejected",
			"reason", "invalid_payload",
			"group_id", wire.ConversationID,
			"event_id", wire.EventID,
			"actor_id", msg.SenderID,
		)
		return
	}

	s.groupRuntime.StateMu.Lock()
	defer s.groupRuntime.StateMu.Unlock()
	svc := &groupdomain.InboundOrchestrationService{
		States:   s.groupRuntime.States,
		EventLog: s.groupRuntime.EventLog,
		Persist: func(states map[string]groupdomain.GroupState, eventLog map[string][]groupdomain.GroupEvent) error {
			if s.groupStateStore == nil {
				return nil
			}
			return s.groupStateStore.Persist(states, eventLog)
		},
		Now:             func() time.Time { return now },
		IdentityID:      func() string { return s.identityManager.GetIdentity().ID },
		IsBlockedSender: s.privacyCore.IsBlockedSender,
		GuardReplay:     s.guardInboundGroupReplay,
		NotifyGroupUpdated: func(event groupdomain.GroupEvent) {
			s.notify("notify.group.updated", map[string]any{
				"group_id":           event.GroupID,
				"event_id":           event.ID,
				"event_type":         event.Type,
				"membership_version": event.Version,
				"actor_id":           event.ActorID,
			})
		},
		RecordError:          s.recordError,
		RecordGroupAggregate: s.recordGroupAggregate,
		Warn:                 s.logger.Warn,
		Debug:                s.logger.Debug,
	}
	svc.HandleInboundGroupEvent(groupdomain.InboundGroupEventParams{
		Event:          event,
		SenderDeviceID: wire.SenderDeviceID,
		HasDevice:      wire.Device != nil,
		DeviceID: func() string {
			if wire.Device == nil {
				return ""
			}
			return wire.Device.ID
		}(),
	})
}

func toInboundPrivateMessage(msg waku.PrivateMessage) messagingapp.InboundPrivateMessage {
	return messagingapp.InboundPrivateMessage{
		ID:        msg.ID,
		SenderID:  msg.SenderID,
		Recipient: msg.Recipient,
		Payload:   append([]byte(nil), msg.Payload...),
	}
}

func (s *Service) notifyGroupUpdated(event groupdomain.GroupEvent) {
	s.notify("notify.group.updated", map[string]any{
		"group_id":           event.GroupID,
		"event_id":           event.ID,
		"event_type":         event.Type,
		"membership_version": event.Version,
		"actor_id":           event.ActorID,
	})
}

func (s *Service) guardInboundGroupReplay(kind, groupID, senderDeviceID, uniqueID string, occurredAt, now time.Time) error {
	key, err := groupdomain.BuildReplayGuardKey(kind, groupID, senderDeviceID, uniqueID)
	if err != nil {
		return err
	}
	if err := groupdomain.ValidateReplayOccurredAt(occurredAt, now); err != nil {
		return err
	}

	cutoff := now.Add(-groupdomain.ReplayWindow)
	s.groupRuntime.ReplayMu.Lock()
	defer s.groupRuntime.ReplayMu.Unlock()
	if s.groupRuntime.ReplaySeen == nil {
		s.groupRuntime.ReplaySeen = make(map[string]time.Time)
	}
	for seenKey, seenAt := range s.groupRuntime.ReplaySeen {
		if seenAt.Before(cutoff) {
			delete(s.groupRuntime.ReplaySeen, seenKey)
		}
	}
	if _, exists := s.groupRuntime.ReplaySeen[key]; exists {
		return groupdomain.ErrOutOfOrderGroupEvent
	}
	s.groupRuntime.ReplaySeen[key] = now
	return nil
}

func (s *Service) evaluateInboundPolicy(senderID string) privacydomain.InboundMessagePolicyDecision {
	return privacydomain.EvaluateInboundMessagePolicy(privacydomain.InboundMessagePolicyInput{
		IsKnownContact: s.identityManager.HasContact(senderID),
		IsBlocked:      s.privacyCore.IsBlockedSender(senderID),
		PrivacyMode:    s.privacyCore.CurrentMode(),
	})
}

func (s *Service) applyInboundReceiptStatus(receiptHandling messagingapp.InboundReceiptHandling) {
	s.updateMessageStatusAndNotify(receiptHandling.MessageID, receiptHandling.Status)
}

func (s *Service) persistInboundMessage(in models.Message, senderID string) bool {
	correlationID := messageCorrelationID(in.ID, in.ContactID)
	if err := s.messageStore.SaveMessage(in); err != nil {
		if errors.Is(err, storage.ErrMessageIDConflict) {
			s.logWarn("message.inbound_conflict", correlationID, "inbound message id conflict ignored", "message_id", in.ID, "contact_id", in.ContactID)
			return false
		}
		s.recordErrorWithContext(contracts.ErrorCategoryStorage, err, "message.inbound_persist", correlationID, "message_id", in.ID, "contact_id", in.ContactID)
		return false
	}
	s.logInfo("message.inbound_received", correlationID, "message received", "message_id", in.ID, "contact_id", in.ContactID, "content_type", in.ContentType)
	s.notify("notify.message.new", map[string]any{
		"contact_id": senderID,
		"message":    in,
	})
	return true
}

func (s *Service) persistInboundRequest(in models.Message) bool {
	correlationID := messageCorrelationID(in.ID, in.ContactID)
	s.requestRuntime.Mu.Lock()

	thread := s.requestRuntime.Inbox[in.ContactID]
	if inboxapp.ThreadHasMessage(thread, in.ID) {
		s.requestRuntime.Mu.Unlock()
		s.logWarn("request.inbound_conflict", correlationID, "inbound request message id conflict ignored", "message_id", in.ID, "contact_id", in.ContactID)
		return false
	}
	in.Timestamp = inboxapp.NormalizeInboundTimestamp(in.Timestamp)
	nextInbox := inboxapp.CopyInboxState(s.requestRuntime.Inbox)
	nextInbox[in.ContactID] = append(nextInbox[in.ContactID], in)
	if err := s.persistRequestInboxSnapshotLocked(nextInbox); err != nil {
		s.requestRuntime.Mu.Unlock()
		s.recordErrorWithContext(contracts.ErrorCategoryStorage, err, "request.inbound_persist", correlationID, "message_id", in.ID, "contact_id", in.ContactID)
		return false
	}
	s.requestRuntime.Inbox = nextInbox
	s.requestRuntime.Mu.Unlock()
	s.notify("notify.request.new", map[string]any{
		"contact_id": in.ContactID,
		"message":    in,
	})
	return true
}
