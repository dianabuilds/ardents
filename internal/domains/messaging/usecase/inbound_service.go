package usecase

import (
	"aim-chat/go-backend/internal/domains/contracts"
	messagingpolicy "aim-chat/go-backend/internal/domains/messaging/policy"
	"aim-chat/go-backend/internal/waku"
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
	ValidateInboundDeviceAuth   func(msg waku.PrivateMessage, wire contracts.WirePayload) error
	ResolveInboundContent       func(msg waku.PrivateMessage, wire contracts.WirePayload) ([]byte, string, error)
	HandleInboundGroupMessage   func(msg waku.PrivateMessage, wire contracts.WirePayload)
	HandleInboundGroupEvent     func(msg waku.PrivateMessage, wire contracts.WirePayload)
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

func (s *InboundService) HandleIncomingPrivateMessage(msg waku.PrivateMessage) {
	decision := s.deps.EvaluateInboundPolicy(msg.SenderID)
	switch decision.Action {
	case InboundPolicyActionReject:
		s.recordErr("crypto", decision.Err)
		return
	case InboundPolicyActionQueue:
		s.HandleInboundMessageRequest(msg)
		return
	}

	content := append([]byte(nil), msg.Payload...)
	contentType := "text"

	var wire contracts.WirePayload
	if err := json.Unmarshal(msg.Payload, &wire); err == nil {
		if err := messagingpolicy.ValidateWirePayload(wire); err != nil {
			s.recordErr("api", err)
			return
		}
		hasCard := wire.Card != nil
		if s.deps.ShouldAutoAddUnknownSender(decision, msg.SenderID, wire.ConversationType, hasCard) {
			if err := s.deps.AddContactByIdentityID(msg.SenderID, msg.SenderID); err != nil {
				s.recordErr("crypto", err)
			}
		} else if violation := s.deps.ValidateInboundContactTrust(msg.SenderID, wire); violation != nil {
			s.recordErr("crypto", violation.Err)
			if s.deps.NotifySecurityAlert != nil {
				s.deps.NotifySecurityAlert(violation.AlertCode, msg.SenderID, violation.Err.Error())
			}
			return
		}

		if wire.Kind == "device_revoke" && wire.Revocation != nil {
			if err := s.deps.ApplyDeviceRevocation(msg.SenderID, *wire.Revocation); err != nil {
				s.recordErr("crypto", err)
			}
			return
		}
		if !s.deps.ShouldBypassInboundDevice(decision, msg.SenderID, wire.ConversationType, hasCard) {
			if err := s.deps.ValidateInboundDeviceAuth(msg, wire); err != nil {
				s.recordErr(ErrorCategory(err), err)
				return
			}
		}
		if wire.ConversationType == models.ConversationTypeGroup {
			if wire.EventType == messagingpolicy.GroupWireEventTypeMessage {
				s.deps.HandleInboundGroupMessage(msg, wire)
				return
			}
			s.deps.HandleInboundGroupEvent(msg, wire)
			return
		}
		receiptHandling := ResolveInboundReceiptHandling(wire)
		if receiptHandling.Handled {
			if receiptHandling.ShouldUpdate && s.deps.ApplyInboundReceiptStatus != nil {
				s.deps.ApplyInboundReceiptStatus(receiptHandling)
			}
			return
		}
		var decryptErr error
		content, contentType, decryptErr = s.deps.ResolveInboundContent(msg, wire)
		if decryptErr != nil {
			s.recordErr("crypto", decryptErr)
		}
	}

	in := BuildInboundStoredMessage(msg, wire.ThreadID, content, contentType, time.Now())
	if !s.deps.PersistInboundMessage(in, msg.SenderID) {
		return
	}
	if !s.deps.HasVerifiedContact(msg.SenderID) {
		return
	}
	if err := s.deps.SendReceiptDelivered(msg.SenderID, msg.ID); err != nil {
		s.recordErr("network", err)
	}
}

func (s *InboundService) HandleInboundMessageRequest(msg waku.PrivateMessage) {
	content := append([]byte(nil), msg.Payload...)
	contentType := "text"

	var wire contracts.WirePayload
	if err := json.Unmarshal(msg.Payload, &wire); err == nil {
		if err := messagingpolicy.ValidateWirePayload(wire); err != nil {
			s.recordErr("api", err)
			return
		}
		receiptHandling := ResolveInboundReceiptHandling(wire)
		if receiptHandling.Handled {
			return
		}
		var decryptErr error
		content, contentType, decryptErr = s.deps.ResolveInboundContent(msg, wire)
		if decryptErr != nil {
			s.recordErr("crypto", decryptErr)
		}
	}

	in := BuildInboundStoredMessage(msg, wire.ThreadID, content, contentType, time.Now())
	if !s.deps.PersistInboundRequest(in) {
		return
	}
	if !s.deps.HasVerifiedContact(msg.SenderID) {
		return
	}
	if err := s.deps.SendReceiptDelivered(msg.SenderID, msg.ID); err != nil {
		s.recordErr("network", err)
	}
}

func (s *InboundService) recordErr(category string, err error) {
	if s.deps.RecordError != nil && err != nil {
		s.deps.RecordError(category, err)
	}
}
