package messaging

import (
	"aim-chat/go-backend/internal/domains/contracts"
	"errors"
	"testing"
	"time"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

type fakeInboundDeviceVerifier struct {
	err error
}

func (f *fakeInboundDeviceVerifier) VerifyInboundDevice(_ string, _ models.Device, _, _ []byte) error {
	return f.err
}

type fakeInboundDecryptor struct {
	plain []byte
	err   error
}

func (f *fakeInboundDecryptor) Decrypt(_ string, _ crypto.MessageEnvelope) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]byte(nil), f.plain...), nil
}

func TestValidateInboundDeviceAuth_MissingDevice(t *testing.T) {
	msg := waku.PrivateMessage{ID: "m1", SenderID: "s1", Recipient: "r1"}
	wire := contracts.WirePayload{Kind: "plain", Plain: []byte("x")}
	err := ValidateInboundDeviceAuth(msg, wire, &fakeInboundDeviceVerifier{})
	if err == nil || ErrorCategory(err) != "crypto" {
		t.Fatalf("expected crypto categorized error, got %v", err)
	}
}

func TestValidateInboundDeviceAuth_VerifierError(t *testing.T) {
	msg := waku.PrivateMessage{ID: "m1", SenderID: "s1", Recipient: "r1"}
	wire := contracts.WirePayload{
		Kind:      "plain",
		Plain:     []byte("x"),
		Device:    &models.Device{ID: "d1"},
		DeviceSig: []byte("sig"),
	}
	err := ValidateInboundDeviceAuth(msg, wire, &fakeInboundDeviceVerifier{err: errors.New("bad sig")})
	if err == nil || ErrorCategory(err) != "crypto" {
		t.Fatalf("expected crypto categorized error, got %v", err)
	}
}

func TestResolveInboundContent_E2EEUnreadable(t *testing.T) {
	msg := waku.PrivateMessage{ID: "m1", SenderID: "s1", Payload: []byte("cipher")}
	wire := contracts.WirePayload{Kind: "e2ee", Envelope: crypto.MessageEnvelope{}}
	content, kind, err := ResolveInboundContent(msg, wire, &fakeInboundDecryptor{err: errors.New("decrypt failed")})
	if err == nil {
		t.Fatal("expected decrypt error")
	}
	if kind != "e2ee-unreadable" {
		t.Fatalf("unexpected content type: %s", kind)
	}
	if string(content) != "cipher" {
		t.Fatalf("expected ciphertext fallback")
	}
}

func TestBuildInboundStoredMessage(t *testing.T) {
	now := time.Now()
	msg := waku.PrivateMessage{ID: "m1", SenderID: "s1"}
	in := BuildInboundStoredMessage(msg, []byte("hello"), "text", now)
	if in.ID != "m1" || in.ContactID != "s1" || in.Direction != "in" || in.Status != "delivered" {
		t.Fatalf("unexpected inbound message: %#v", in)
	}
	if in.ConversationType != models.ConversationTypeDirect || in.ConversationID != "s1" {
		t.Fatalf("unexpected inbound conversation fields: type=%q id=%q", in.ConversationType, in.ConversationID)
	}
}

func TestBuildInboundGroupStoredMessage(t *testing.T) {
	now := time.Now()
	msg := waku.PrivateMessage{ID: "gm1", SenderID: "s1"}
	in := BuildInboundGroupStoredMessage(msg, "group-1", []byte("hello group"), "e2ee", now)
	if in.ID != "gm1" || in.ContactID != "s1" || in.Direction != "in" || in.Status != "delivered" {
		t.Fatalf("unexpected inbound group message: %#v", in)
	}
	if in.ConversationType != models.ConversationTypeGroup || in.ConversationID != "group-1" {
		t.Fatalf("unexpected inbound group conversation fields: type=%q id=%q", in.ConversationType, in.ConversationID)
	}
}
