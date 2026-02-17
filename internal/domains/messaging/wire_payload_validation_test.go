package messaging_test

import (
	"aim-chat/go-backend/internal/domains/contracts"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateWirePayloadBackwardCompatibleDirect(t *testing.T) {
	directLegacy := contracts.WirePayload{
		Kind:  "plain",
		Plain: []byte("hello"),
	}
	if err := messagingapp.ValidateWirePayload(directLegacy); err != nil {
		t.Fatalf("legacy direct payload must be valid, got %v", err)
	}

	directExplicit := contracts.WirePayload{
		Kind:             "e2ee",
		ConversationType: "direct",
		ConversationID:   "aim1contact",
	}
	if err := messagingapp.ValidateWirePayload(directExplicit); err != nil {
		t.Fatalf("explicit direct payload must be valid, got %v", err)
	}
}

func TestValidateWirePayloadRejectsInvalidGroupPayload(t *testing.T) {
	cases := []struct {
		name string
		wire contracts.WirePayload
	}{
		{
			name: "group type missing conversation id",
			wire: contracts.WirePayload{
				Kind:              "plain",
				ConversationType:  "group",
				EventID:           "evt-1",
				EventType:         "member_add",
				MembershipVersion: 1,
				SenderDeviceID:    "dev-1",
			},
		},
		{
			name: "group metadata without group type",
			wire: contracts.WirePayload{
				Kind:              "plain",
				ConversationID:    "group-1",
				EventID:           "evt-1",
				EventType:         "member_add",
				MembershipVersion: 1,
				SenderDeviceID:    "dev-1",
			},
		},
		{
			name: "invalid event type",
			wire: contracts.WirePayload{
				Kind:              "plain",
				ConversationType:  "group",
				ConversationID:    "group-1",
				EventID:           "evt-1",
				EventType:         "invalid_event",
				MembershipVersion: 1,
				SenderDeviceID:    "dev-1",
			},
		},
		{
			name: "invalid membership version",
			wire: contracts.WirePayload{
				Kind:             "plain",
				ConversationType: "group",
				ConversationID:   "group-1",
				EventID:          "evt-1",
				EventType:        "member_add",
				SenderDeviceID:   "dev-1",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := messagingapp.ValidateWirePayload(tc.wire)
			if !errors.Is(err, messagingapp.ErrInvalidGroupWirePayload) {
				t.Fatalf("expected messagingapp.ErrInvalidGroupWirePayload, got %v", err)
			}
		})
	}
}

func TestWirePayloadGroupSerializationAndValidationContract(t *testing.T) {
	wire := contracts.WirePayload{
		Kind:              "plain",
		Plain:             []byte("group-event"),
		ConversationType:  "group",
		ConversationID:    "group-1",
		EventID:           "evt-7",
		EventType:         "member_add",
		MembershipVersion: 7,
		SenderDeviceID:    "device-1",
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal wire failed: %v", err)
	}
	var decoded contracts.WirePayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal wire failed: %v", err)
	}
	if err := messagingapp.ValidateWirePayload(decoded); err != nil {
		t.Fatalf("decoded wire must stay valid, got %v", err)
	}
	if decoded.ConversationID != wire.ConversationID || decoded.EventID != wire.EventID {
		t.Fatalf("roundtrip mismatch: decoded=%+v", decoded)
	}
}

func TestValidateWirePayloadAcceptsGroupMessageEventType(t *testing.T) {
	wire := contracts.WirePayload{
		Kind:              "e2ee",
		ConversationType:  "group",
		ConversationID:    "group-1",
		EventID:           "evt-msg-1",
		EventType:         messagingapp.GroupWireEventTypeMessage,
		MembershipVersion: 3,
		GroupKeyVersion:   1,
		SenderDeviceID:    "device-1",
	}
	if err := messagingapp.ValidateWirePayload(wire); err != nil {
		t.Fatalf("group message wire must be valid, got %v", err)
	}
}

func TestValidateWirePayloadRejectsGroupMessageWithoutKeyVersion(t *testing.T) {
	wire := contracts.WirePayload{
		Kind:              "e2ee",
		ConversationType:  "group",
		ConversationID:    "group-1",
		EventID:           "evt-msg-1",
		EventType:         messagingapp.GroupWireEventTypeMessage,
		MembershipVersion: 3,
		SenderDeviceID:    "device-1",
	}
	err := messagingapp.ValidateWirePayload(wire)
	if !errors.Is(err, messagingapp.ErrInvalidGroupWirePayload) {
		t.Fatalf("expected messagingapp.ErrInvalidGroupWirePayload, got %v", err)
	}
}

func TestBuildWireAuthPayloadRejectsInvalidGroupPayload(t *testing.T) {
	_, err := messagingapp.BuildWireAuthPayload("m1", "sender", "recipient", contracts.WirePayload{
		Kind:             "plain",
		ConversationType: "group",
		ConversationID:   "group-1",
		EventID:          "evt-1",
		EventType:        "member_add",
		// missing membership_version and sender_device_id
	})
	if !errors.Is(err, messagingapp.ErrInvalidGroupWirePayload) {
		t.Fatalf("expected messagingapp.ErrInvalidGroupWirePayload, got %v", err)
	}
}
