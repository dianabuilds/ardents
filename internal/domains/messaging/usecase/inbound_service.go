package usecase

import (
	"aim-chat/go-backend/internal/domains/contracts"
	messagingpolicy "aim-chat/go-backend/internal/domains/messaging/policy"
	"aim-chat/go-backend/pkg/models"
	"encoding/json"
	"time"
)

type InboundPolicyAction string

const (
	InboundPolicyActionReject InboundPolicyAction = "reject"
	InboundPolicyActionAccept InboundPolicyAction = "accept"
	InboundPolicyActionQueue  InboundPolicyAction = "queue"
)

type InboundPolicyDecision struct {
	Action InboundPolicyAction
	Reason string
	Err    error
}

type InboundServiceDeps struct {
	EvaluateInboundPolicy       func(senderID string) InboundPolicyDecision
	ShouldAutoAddUnknownSender  func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool
	ShouldBypassInboundDevice   func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool
	HasVerifiedContact          func(senderID string) bool
	AddContactByIdentityID      func(contactID, displayName string) error
	ValidateInboundContactTrust func(senderID string, wire contracts.WirePayload) *InboundContactTrustViolation
	NotifySecurityAlert         func(kind, contactID, message string)
	ApplyDeviceRevocation       func(senderID string, rev models.DeviceRevocation) error
	ValidateInboundDeviceAuth   func(msg InboundPrivateMessage, wire contracts.WirePayload) error
	ResolveInboundContent       func(msg InboundPrivateMessage, wire contracts.WirePayload) ([]byte, string, error)
	HandleInboundGroupMessage   func(msg InboundPrivateMessage, wire contracts.WirePayload)
	HandleInboundGroupEvent     func(msg InboundPrivateMessage, wire contracts.WirePayload)
	ApplyInboundReceiptStatus   func(receiptHandling InboundReceiptHandling)
	PersistInboundMessage       func(in models.Message, senderID string) bool
	PersistInboundRequest       func(in models.Message) bool
	SendReceiptDelivered        func(senderID, messageID string) error
	RecordError                 func(category string, err error)
}

type InboundService struct {
	deps InboundServiceDeps
}

func NewInboundService(deps InboundServiceDeps) *InboundService {
	return &InboundService{deps: deps}
}

func (s *InboundService) HandleIncomingPrivateMessage(msg InboundPrivateMessage) {
	content := append([]byte(nil), msg.Payload...)
	contentType := "text"
	decision, shouldStop := s.evaluateInboundPolicy(msg)
	if shouldStop {
		return
	}
	wire, handled := s.processInboundWire(msg, decision, &content, &contentType)
	if handled {
		return
	}
	s.persistInboundMessageAndReceipt(msg, wire.ThreadID, content, contentType)
}

func (s *InboundService) evaluateInboundPolicy(msg InboundPrivateMessage) (InboundPolicyDecision, bool) {
	decision := s.deps.EvaluateInboundPolicy(msg.SenderID)
	switch decision.Action {
	case InboundPolicyActionReject:
		s.recordErr(contracts.ErrorCategoryCrypto, decision.Err)
		return decision, true
	case InboundPolicyActionQueue:
		s.HandleInboundMessageRequest(msg)
		return decision, true
	default:
		return decision, false
	}
}

func (s *InboundService) decodeInboundWire(msg InboundPrivateMessage) (wire contracts.WirePayload, parsed bool, valid bool) {
	if err := json.Unmarshal(msg.Payload, &wire); err != nil {
		return contracts.WirePayload{}, false, false
	}
	if err := messagingpolicy.ValidateWirePayload(wire); err != nil {
		s.recordErr(contracts.ErrorCategoryAPI, err)
		return contracts.WirePayload{}, true, false
	}
	return wire, true, true
}

func (s *InboundService) processInboundWire(
	msg InboundPrivateMessage,
	decision InboundPolicyDecision,
	content *[]byte,
	contentType *string,
) (contracts.WirePayload, bool) {
	wire, parsed, valid := s.decodeInboundWire(msg)
	if !parsed {
		return contracts.WirePayload{}, false
	}
	if !valid {
		return contracts.WirePayload{}, true
	}
	hasCard := wire.Card != nil
	if s.deps.ShouldAutoAddUnknownSender(decision, msg.SenderID, wire.ConversationType, hasCard) {
		if err := s.deps.AddContactByIdentityID(msg.SenderID, msg.SenderID); err != nil {
			s.recordErr(contracts.ErrorCategoryCrypto, err)
		}
	} else if violation := s.deps.ValidateInboundContactTrust(msg.SenderID, wire); violation != nil {
		s.recordErr(contracts.ErrorCategoryCrypto, violation.Err)
		if s.deps.NotifySecurityAlert != nil {
			s.deps.NotifySecurityAlert(violation.AlertCode, msg.SenderID, violation.Err.Error())
		}
		return contracts.WirePayload{}, true
	}
	if wire.Kind == "device_revoke" && wire.Revocation != nil {
		if err := s.deps.ApplyDeviceRevocation(msg.SenderID, *wire.Revocation); err != nil {
			s.recordErr(contracts.ErrorCategoryCrypto, err)
		}
		return contracts.WirePayload{}, true
	}
	if !s.deps.ShouldBypassInboundDevice(decision, msg.SenderID, wire.ConversationType, hasCard) {
		if err := s.deps.ValidateInboundDeviceAuth(msg, wire); err != nil {
			s.recordErr(ErrorCategory(err), err)
			return contracts.WirePayload{}, true
		}
	}
	if wire.ConversationType == models.ConversationTypeGroup {
		if wire.EventType == messagingpolicy.GroupWireEventTypeMessage {
			s.deps.HandleInboundGroupMessage(msg, wire)
		} else {
			s.deps.HandleInboundGroupEvent(msg, wire)
		}
		return contracts.WirePayload{}, true
	}
	receiptHandling := ResolveInboundReceiptHandling(wire)
	if receiptHandling.Handled {
		if receiptHandling.ShouldUpdate && s.deps.ApplyInboundReceiptStatus != nil {
			s.deps.ApplyInboundReceiptStatus(receiptHandling)
		}
		return contracts.WirePayload{}, true
	}
	resolvedContent, resolvedType, decryptErr := s.deps.ResolveInboundContent(msg, wire)
	if decryptErr != nil {
		s.recordErr(contracts.ErrorCategoryCrypto, decryptErr)
	}
	*content = resolvedContent
	*contentType = resolvedType
	return wire, false
}

func (s *InboundService) persistInboundMessageAndReceipt(
	msg InboundPrivateMessage,
	threadID string,
	content []byte,
	contentType string,
) {
	s.persistInboundAndSendReceipt(msg, threadID, content, contentType, func(in models.Message) bool {
		return s.deps.PersistInboundMessage(in, msg.SenderID)
	})
}

func (s *InboundService) persistInboundAndSendReceipt(
	msg InboundPrivateMessage,
	threadID string,
	content []byte,
	contentType string,
	persist func(models.Message) bool,
) {
	if persist == nil {
		return
	}
	in := BuildInboundStoredMessage(msg, threadID, content, contentType, time.Now())
	if !persist(in) {
		return
	}
	if !s.deps.HasVerifiedContact(msg.SenderID) {
		return
	}
	if err := s.deps.SendReceiptDelivered(msg.SenderID, msg.ID); err != nil {
		s.recordErr(contracts.ErrorCategoryNetwork, err)
	}
}

func (s *InboundService) HandleInboundMessageRequest(msg InboundPrivateMessage) {
	content := append([]byte(nil), msg.Payload...)
	contentType := "text"

	wire, parsed, valid := s.decodeInboundWire(msg)
	if parsed {
		if !valid {
			return
		}
		receiptHandling := ResolveInboundReceiptHandling(wire)
		if receiptHandling.Handled {
			return
		}
		var decryptErr error
		content, contentType, decryptErr = s.deps.ResolveInboundContent(msg, wire)
		if decryptErr != nil {
			s.recordErr(contracts.ErrorCategoryCrypto, decryptErr)
		}
	}
	s.persistInboundAndSendReceipt(msg, wire.ThreadID, content, contentType, s.deps.PersistInboundRequest)
}

func (s *InboundService) recordErr(category string, err error) {
	if s.deps.RecordError != nil && err != nil {
		s.deps.RecordError(category, err)
	}
}
