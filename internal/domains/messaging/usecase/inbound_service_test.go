package usecase

import (
	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/pkg/models"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func defaultInboundDeps() InboundServiceDeps {
	return InboundServiceDeps{
		EvaluateInboundPolicy: func(senderID string) InboundPolicyDecision {
			return InboundPolicyDecision{Action: InboundPolicyActionAccept}
		},
		ShouldAutoAddUnknownSender: func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
			return false
		},
		ShouldBypassInboundDevice: func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
			return true
		},
		HasVerifiedContact:          func(senderID string) bool { return false },
		AddContactByIdentityID:      func(contactID, displayName string) error { return nil },
		ValidateInboundContactTrust: func(senderID string, wire contracts.WirePayload) *InboundContactTrustViolation { return nil },
		NotifySecurityAlert:         func(kind, contactID, message string) {},
		ApplyDeviceRevocation:       func(senderID string, rev models.DeviceRevocation) error { return nil },
		ValidateInboundDeviceAuth:   func(msg InboundPrivateMessage, wire contracts.WirePayload) error { return nil },
		ResolveInboundContent: func(msg InboundPrivateMessage, wire contracts.WirePayload) ([]byte, string, error) {
			return append([]byte(nil), msg.Payload...), "text", nil
		},
		HandleInboundGroupMessage: func(msg InboundPrivateMessage, wire contracts.WirePayload) {},
		HandleInboundGroupEvent:   func(msg InboundPrivateMessage, wire contracts.WirePayload) {},
		ApplyInboundReceiptStatus: func(receiptHandling InboundReceiptHandling) {},
		PersistInboundMessage:     func(in models.Message, senderID string) bool { return true },
		PersistInboundRequest:     func(in models.Message) bool { return true },
		SendReceiptDelivered:      func(senderID, messageID string) error { return nil },
		RecordError:               func(category string, err error) {},
	}
}

func mustMarshalWirePayload(t *testing.T, wire contracts.WirePayload) []byte {
	t.Helper()
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func TestInboundService_RejectPolicyStopsProcessing(t *testing.T) {
	rejectErr := errors.New("rejected")
	recorded := make([]string, 0, 1)
	persistCalled := false
	service := NewInboundService(InboundServiceDeps{
		EvaluateInboundPolicy: func(senderID string) InboundPolicyDecision {
			return InboundPolicyDecision{Action: InboundPolicyActionReject, Err: rejectErr}
		},
		PersistInboundMessage: func(in models.Message, senderID string) bool {
			persistCalled = true
			return true
		},
		RecordError: func(category string, err error) {
			recorded = append(recorded, category+":"+err.Error())
		},
	})

	service.HandleIncomingPrivateMessage(InboundPrivateMessage{ID: "m1", SenderID: "alice", Payload: []byte("raw")})

	if persistCalled {
		t.Fatalf("persist should not be called when policy rejects message")
	}
	if len(recorded) != 1 || recorded[0] != "crypto:rejected" {
		t.Fatalf("unexpected recorded errors: %+v", recorded)
	}
}

func TestInboundService_ReceiptUpdateHandledWithoutPersist(t *testing.T) {
	now := time.Now().UTC()
	payload, err := json.Marshal(contracts.WirePayload{
		Kind: "receipt",
		Receipt: &models.MessageReceipt{
			MessageID: "msg-42",
			Status:    "delivered",
			Timestamp: now,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	persistCalled := false
	applied := false
	service := NewInboundService(InboundServiceDeps{
		EvaluateInboundPolicy: func(senderID string) InboundPolicyDecision {
			return InboundPolicyDecision{Action: InboundPolicyActionAccept}
		},
		ShouldAutoAddUnknownSender: func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
			return false
		},
		ValidateInboundContactTrust: func(senderID string, wire contracts.WirePayload) *InboundContactTrustViolation {
			return nil
		},
		ShouldBypassInboundDevice: func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
			return true
		},
		ResolveInboundContent: func(msg InboundPrivateMessage, wire contracts.WirePayload) ([]byte, string, error) {
			return []byte("should-not-run"), "text", nil
		},
		ApplyInboundReceiptStatus: func(receiptHandling InboundReceiptHandling) {
			applied = receiptHandling.ShouldUpdate && receiptHandling.MessageID == "msg-42" && receiptHandling.Status == "delivered"
		},
		PersistInboundMessage: func(in models.Message, senderID string) bool {
			persistCalled = true
			return true
		},
	})

	service.HandleIncomingPrivateMessage(InboundPrivateMessage{ID: "m2", SenderID: "alice", Payload: payload})

	if !applied {
		t.Fatalf("expected receipt status update to be applied")
	}
	if persistCalled {
		t.Fatalf("persist should not run for handled receipt wire")
	}
}

func TestInboundService_NonWirePayloadPersistsAndSendsReceipt(t *testing.T) {
	persisted := models.Message{}
	receiptSent := false
	service := NewInboundService(InboundServiceDeps{
		EvaluateInboundPolicy: func(senderID string) InboundPolicyDecision {
			return InboundPolicyDecision{Action: InboundPolicyActionAccept}
		},
		ShouldAutoAddUnknownSender: func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
			return false
		},
		ValidateInboundContactTrust: func(senderID string, wire contracts.WirePayload) *InboundContactTrustViolation {
			return nil
		},
		ShouldBypassInboundDevice: func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
			return true
		},
		ResolveInboundContent: func(msg InboundPrivateMessage, wire contracts.WirePayload) ([]byte, string, error) {
			return []byte("should-not-run"), "text", nil
		},
		PersistInboundMessage: func(in models.Message, senderID string) bool {
			persisted = in
			return true
		},
		HasVerifiedContact: func(senderID string) bool { return true },
		SendReceiptDelivered: func(senderID, messageID string) error {
			receiptSent = senderID == "alice" && messageID == "m3"
			return nil
		},
	})

	service.HandleIncomingPrivateMessage(InboundPrivateMessage{ID: "m3", SenderID: "alice", Payload: []byte("raw payload")})

	if string(persisted.Content) != "raw payload" {
		t.Fatalf("unexpected stored content: %q", string(persisted.Content))
	}
	if persisted.ContentType != "text" {
		t.Fatalf("unexpected content type: %s", persisted.ContentType)
	}
	if !receiptSent {
		t.Fatalf("expected delivery receipt to be sent")
	}
}

func TestInboundService_QueuePolicyRoutesToRequestFlow(t *testing.T) {
	deps := defaultInboundDeps()
	persistMessageCalled := false
	persistRequestCalled := false
	deps.EvaluateInboundPolicy = func(senderID string) InboundPolicyDecision {
		return InboundPolicyDecision{Action: InboundPolicyActionQueue}
	}
	deps.PersistInboundMessage = func(in models.Message, senderID string) bool {
		persistMessageCalled = true
		return true
	}
	deps.PersistInboundRequest = func(in models.Message) bool {
		persistRequestCalled = true
		return true
	}
	service := NewInboundService(deps)

	service.HandleIncomingPrivateMessage(InboundPrivateMessage{ID: "m4", SenderID: "alice", Payload: []byte("queued raw")})

	if persistMessageCalled {
		t.Fatalf("direct message persistence should not run for queued flow")
	}
	if !persistRequestCalled {
		t.Fatalf("expected queued flow to persist inbound request")
	}
}

func TestInboundService_TrustViolationStopsAndAlerts(t *testing.T) {
	deps := defaultInboundDeps()
	persistCalled := false
	alerted := false
	deps.ValidateInboundContactTrust = func(senderID string, wire contracts.WirePayload) *InboundContactTrustViolation {
		return &InboundContactTrustViolation{
			AlertCode: "contact_card_verification_failed",
			Err:       errors.New("trust violation"),
		}
	}
	deps.NotifySecurityAlert = func(kind, contactID, message string) {
		alerted = kind == "contact_card_verification_failed" && contactID == "alice"
	}
	deps.PersistInboundMessage = func(in models.Message, senderID string) bool {
		persistCalled = true
		return true
	}
	service := NewInboundService(deps)

	service.HandleIncomingPrivateMessage(InboundPrivateMessage{
		ID:       "m5",
		SenderID: "alice",
		Payload: mustMarshalWirePayload(t, contracts.WirePayload{
			Kind: "plain",
		}),
	})

	if !alerted {
		t.Fatalf("expected security alert to be emitted on trust violation")
	}
	if persistCalled {
		t.Fatalf("message must not be persisted when trust validation fails")
	}
}

func TestInboundService_DeviceRevokeAppliesRevocation(t *testing.T) {
	deps := defaultInboundDeps()
	revocationApplied := false
	persistCalled := false
	deps.ApplyDeviceRevocation = func(senderID string, rev models.DeviceRevocation) error {
		revocationApplied = senderID == "alice" && rev.DeviceID == "device-1"
		return nil
	}
	deps.PersistInboundMessage = func(in models.Message, senderID string) bool {
		persistCalled = true
		return true
	}
	service := NewInboundService(deps)

	service.HandleIncomingPrivateMessage(InboundPrivateMessage{
		ID:       "m6",
		SenderID: "alice",
		Payload: mustMarshalWirePayload(t, contracts.WirePayload{
			Kind: "device_revoke",
			Revocation: &models.DeviceRevocation{
				IdentityID: "alice",
				DeviceID:   "device-1",
				Timestamp:  time.Now().UTC(),
			},
		}),
	})

	if !revocationApplied {
		t.Fatalf("expected device revocation to be applied")
	}
	if persistCalled {
		t.Fatalf("message should not be persisted for device revocation control message")
	}
}

func TestInboundService_GroupMessageRoutesToGroupHandler(t *testing.T) {
	deps := defaultInboundDeps()
	groupMessageCalled := false
	groupEventCalled := false
	persistCalled := false
	deps.HandleInboundGroupMessage = func(msg InboundPrivateMessage, wire contracts.WirePayload) {
		groupMessageCalled = true
	}
	deps.HandleInboundGroupEvent = func(msg InboundPrivateMessage, wire contracts.WirePayload) {
		groupEventCalled = true
	}
	deps.PersistInboundMessage = func(in models.Message, senderID string) bool {
		persistCalled = true
		return true
	}
	service := NewInboundService(deps)

	service.HandleIncomingPrivateMessage(InboundPrivateMessage{
		ID:       "m7",
		SenderID: "alice",
		Payload: mustMarshalWirePayload(t, contracts.WirePayload{
			Kind:              "plain",
			ConversationType:  models.ConversationTypeGroup,
			ConversationID:    "group-1",
			EventID:           "evt-1",
			EventType:         "message",
			MembershipVersion: 2,
			GroupKeyVersion:   1,
			SenderDeviceID:    "device-1",
		}),
	})

	if !groupMessageCalled || groupEventCalled {
		t.Fatalf("expected only group message handler to be called")
	}
	if persistCalled {
		t.Fatalf("group wire should be handled by group orchestration, not direct persistence")
	}
}

func TestInboundService_DeviceAuthFailureStopsProcessing(t *testing.T) {
	deps := defaultInboundDeps()
	recorded := make([]string, 0, 1)
	persistCalled := false
	deps.ShouldBypassInboundDevice = func(decision InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
		return false
	}
	deps.ValidateInboundDeviceAuth = func(msg InboundPrivateMessage, wire contracts.WirePayload) error {
		return &contracts.CategorizedError{Category: "crypto", Err: errors.New("bad signature")}
	}
	deps.RecordError = func(category string, err error) {
		recorded = append(recorded, category)
	}
	deps.PersistInboundMessage = func(in models.Message, senderID string) bool {
		persistCalled = true
		return true
	}
	service := NewInboundService(deps)

	service.HandleIncomingPrivateMessage(InboundPrivateMessage{
		ID:       "m8",
		SenderID: "alice",
		Payload: mustMarshalWirePayload(t, contracts.WirePayload{
			Kind: "plain",
		}),
	})

	if len(recorded) == 0 || recorded[0] != "crypto" {
		t.Fatalf("expected crypto category error, got: %+v", recorded)
	}
	if persistCalled {
		t.Fatalf("message should not be persisted when device auth fails")
	}
}

func TestInboundService_RequestFlowInvalidWireStopsWithAPIError(t *testing.T) {
	deps := defaultInboundDeps()
	recorded := make([]string, 0, 1)
	persistRequestCalled := false
	deps.RecordError = func(category string, err error) {
		recorded = append(recorded, category)
	}
	deps.PersistInboundRequest = func(in models.Message) bool {
		persistRequestCalled = true
		return true
	}
	service := NewInboundService(deps)

	service.HandleInboundMessageRequest(InboundPrivateMessage{
		ID:       "m9",
		SenderID: "alice",
		Payload: mustMarshalWirePayload(t, contracts.WirePayload{
			Kind:             "plain",
			ConversationType: models.ConversationTypeGroup,
		}),
	})

	if len(recorded) == 0 || recorded[0] != "api" {
		t.Fatalf("expected API validation error to be recorded, got: %+v", recorded)
	}
	if persistRequestCalled {
		t.Fatalf("invalid wire payload should not be persisted in request flow")
	}
}
