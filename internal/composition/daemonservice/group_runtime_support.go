package daemonservice

import (
	groupdomain "aim-chat/go-backend/internal/domains/group"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/pkg/models"
	"errors"
	"strings"
	"time"
)

func (s *Service) groupUseCases() *groupdomain.Service {
	return &groupdomain.Service{
		IdentityID:        func() string { return s.identityManager.GetIdentity().ID },
		WithMembership:    s.withGroupMembership,
		SnapshotStates:    s.snapshotGroupStates,
		GenerateID:        runtimeapp.GeneratePrefixedID,
		GenerateEventID:   s.mustGenerateEventID,
		Now:               time.Now,
		Abuse:             s.groupAbuse,
		IsBlockedSender:   s.privacyCore.IsBlockedSender,
		ActiveDeviceID:    s.activeDeviceID,
		GetMessage:        s.messageStore.GetMessage,
		SaveMessage:       s.messageStore.SaveMessage,
		DeleteMessage:     s.messageStore.DeleteMessage,
		ListMessages:      s.messageStore.ListMessagesByConversation,
		PrepareAndPublish: s.prepareAndPublishGroupMessage,
		RecordError:       s.recordError,
		Notify:            s.notify,
		RecordAggregate:   s.recordGroupAggregate,
		LogInfo:           s.logger.Info,
	}
}

func (s *Service) withGroupMembership(fn func(ms *groupdomain.MembershipService) error) error {
	s.groupRuntime.StateMu.Lock()
	defer s.groupRuntime.StateMu.Unlock()
	return fn(s.groupMembershipServiceLocked())
}

func (s *Service) snapshotGroupStates() map[string]groupdomain.GroupState {
	s.groupRuntime.StateMu.RLock()
	defer s.groupRuntime.StateMu.RUnlock()
	out := make(map[string]groupdomain.GroupState, len(s.groupRuntime.States))
	for groupID, state := range s.groupRuntime.States {
		out[groupID] = groupdomain.CloneState(state)
	}
	return out
}

func (s *Service) groupMembershipServiceLocked() *groupdomain.MembershipService {
	return &groupdomain.MembershipService{
		States:   s.groupRuntime.States,
		EventLog: s.groupRuntime.EventLog,
		Persist: func(states map[string]groupdomain.GroupState, eventLog map[string][]groupdomain.GroupEvent) error {
			if s.groupStateStore == nil {
				return nil
			}
			return s.groupStateStore.Persist(states, eventLog)
		},
		Notify:          s.notifyGroupUpdated,
		GenerateEventID: s.mustGenerateEventID,
	}
}

func (s *Service) mustGenerateEventID() string {
	eventID, err := runtimeapp.GeneratePrefixedID("gevt")
	if err != nil {
		return "gevt_fallback_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return eventID
}

func (s *Service) prepareAndPublishGroupMessage(msg models.Message, recipientID string, meta groupdomain.GroupMessageWireMeta) (string, string, error) {
	wire, _, err := messagingapp.BuildWireForOutboundMessage(msg, s.sessionManager)
	if err != nil {
		return "", messagingapp.ErrorCategory(err), err
	}
	wire.ConversationType = models.ConversationTypeGroup
	wire.ConversationID = meta.GroupID
	wire.EventType = messagingapp.GroupWireEventTypeMessage
	wire.EventID = meta.EventID
	wire.MembershipVersion = meta.MembershipVersion
	wire.GroupKeyVersion = meta.GroupKeyVersion
	wire.SenderDeviceID = meta.SenderDeviceID

	sentID, err := s.publishQueuedMessage(msg, recipientID, wire)
	if err != nil {
		return "", "", err
	}
	return sentID, "", nil
}

func (s *Service) activeDeviceID() (string, error) {
	device, _, err := s.identityManager.ActiveDeviceAuth([]byte("group-device-id"))
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(device.ID)
	if id == "" {
		return "", errors.New("active device id is empty")
	}
	return id, nil
}
