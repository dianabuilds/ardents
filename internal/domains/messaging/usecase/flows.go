package usecase

import (
	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/domains/contracts"
	messagingpolicy "aim-chat/go-backend/internal/domains/messaging/policy"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type inboundContactTrustAccess interface {
	HasVerifiedContact(contactID string) bool
	VerifyContactCard(card models.ContactCard) (bool, error)
	ContactPublicKey(contactID string) ([]byte, bool)
	AddContact(card models.ContactCard) error
}

type InboundContactTrustViolation struct {
	AlertCode string
	Err       error
}

func ValidateInboundContactTrust(senderID string, wire contracts.WirePayload, identity inboundContactTrustAccess) *InboundContactTrustViolation {
	if identity.HasVerifiedContact(senderID) && wire.Card != nil {
		if wire.Card.IdentityID != senderID {
			return &InboundContactTrustViolation{AlertCode: "contact_card_identity_mismatch", Err: errors.New("contact card identity mismatch")}
		}
		ok, err := identity.VerifyContactCard(*wire.Card)
		if err != nil || !ok {
			if err == nil {
				err = errors.New("contact card verification failed")
			}
			return &InboundContactTrustViolation{AlertCode: "contact_card_verification_failed", Err: err}
		}
		if pinnedKey, exists := identity.ContactPublicKey(senderID); exists && !bytes.Equal(pinnedKey, wire.Card.PublicKey) {
			return &InboundContactTrustViolation{AlertCode: "contact_key_pin_mismatch", Err: errors.New("contact public key changed for verified contact")}
		}
	}

	if !identity.HasVerifiedContact(senderID) {
		if wire.Card == nil {
			return &InboundContactTrustViolation{AlertCode: "unverified_sender_missing_card", Err: errors.New("unverified sender did not provide contact card")}
		}
		if wire.Card.IdentityID != senderID {
			return &InboundContactTrustViolation{AlertCode: "contact_card_identity_mismatch", Err: errors.New("contact card identity mismatch")}
		}
		ok, err := identity.VerifyContactCard(*wire.Card)
		if err != nil || !ok {
			if err == nil {
				err = errors.New("contact card verification failed")
			}
			return &InboundContactTrustViolation{AlertCode: "contact_card_verification_failed", Err: err}
		}
		if err := identity.AddContact(*wire.Card); err != nil {
			return &InboundContactTrustViolation{AlertCode: "contact_key_pin_mismatch", Err: err}
		}
	}

	return nil
}

type inboundDeviceVerifier interface {
	VerifyInboundDevice(contactID string, device models.Device, payload, sig []byte) error
}

type inboundSessionDecryptor interface {
	Decrypt(contactID string, env crypto.MessageEnvelope) ([]byte, error)
}

func ValidateInboundDeviceAuth(msg waku.PrivateMessage, wire contracts.WirePayload, identity inboundDeviceVerifier) error {
	authPayload, err := BuildWireAuthPayload(msg.ID, msg.SenderID, msg.Recipient, wire)
	if err != nil {
		return &contracts.CategorizedError{Category: "api", Err: err}
	}
	if wire.Device == nil || len(wire.DeviceSig) == 0 {
		return &contracts.CategorizedError{Category: "crypto", Err: errors.New("missing device authentication")}
	}
	if err := identity.VerifyInboundDevice(msg.SenderID, *wire.Device, authPayload, wire.DeviceSig); err != nil {
		return &contracts.CategorizedError{Category: "crypto", Err: err}
	}
	return nil
}

func ShouldApplyReceiptStatus(status string) bool {
	return status == "delivered" || status == "read"
}

func ResolveInboundContent(msg waku.PrivateMessage, wire contracts.WirePayload, sessions inboundSessionDecryptor) (content []byte, contentType string, decryptErr error) {
	content = append([]byte(nil), msg.Payload...)
	contentType = "text"
	switch wire.Kind {
	case "plain":
		content = append([]byte(nil), wire.Plain...)
		contentType = "text"
	case "e2ee":
		plain, err := sessions.Decrypt(msg.SenderID, wire.Envelope)
		if err != nil {
			return append([]byte(nil), msg.Payload...), "e2ee-unreadable", err
		}
		return plain, "e2ee", nil
	}
	return content, contentType, nil
}

func BuildInboundStoredMessage(msg waku.PrivateMessage, threadID string, content []byte, contentType string, now time.Time) models.Message {
	return models.Message{ID: msg.ID, ContactID: msg.SenderID, ConversationID: msg.SenderID, ConversationType: models.ConversationTypeDirect, ThreadID: strings.TrimSpace(threadID), Content: content, Timestamp: now.UTC(), Direction: "in", Status: "delivered", ContentType: contentType}
}

func BuildInboundGroupStoredMessage(msg waku.PrivateMessage, conversationID, threadID string, content []byte, contentType string, now time.Time) models.Message {
	return models.Message{ID: msg.ID, ContactID: msg.SenderID, ConversationID: strings.TrimSpace(conversationID), ConversationType: models.ConversationTypeGroup, ThreadID: strings.TrimSpace(threadID), Content: content, Timestamp: now.UTC(), Direction: "in", Status: "delivered", ContentType: contentType}
}

type InboundReceiptHandling struct {
	Handled      bool
	ShouldUpdate bool
	MessageID    string
	Status       string
}

type PendingMessage struct {
	Message    models.Message
	RetryCount int
	NextRetry  time.Time
	LastError  string
}

func ResolveInboundReceiptHandling(wire contracts.WirePayload) InboundReceiptHandling {
	if wire.Kind != "receipt" || wire.Receipt == nil {
		return InboundReceiptHandling{}
	}
	h := InboundReceiptHandling{Handled: true}
	if ShouldApplyReceiptStatus(wire.Receipt.Status) {
		h.ShouldUpdate = true
		h.MessageID = wire.Receipt.MessageID
		h.Status = wire.Receipt.Status
	}
	return h
}

func AllocateOutboundMessage(
	contactID, content string,
	threadID string,
	now func() time.Time,
	nextID func() (string, error),
	save func(models.Message) error,
	isMessageIDConflict func(error) bool,
) (models.Message, error) {
	for i := 0; i < 3; i++ {
		msgID, err := nextID()
		if err != nil {
			return models.Message{}, err
		}
		msg := messagingpolicy.NewOutboundMessage(msgID, contactID, content, now())
		msg.ThreadID = strings.TrimSpace(threadID)
		if err := save(msg); err != nil {
			if isMessageIDConflict != nil && isMessageIDConflict(err) {
				continue
			}
			return models.Message{}, err
		}
		return msg, nil
	}
	return models.Message{}, errors.New("failed to allocate unique message id")
}

func NewPlainWire(content []byte) contracts.WirePayload {
	return contracts.WirePayload{Kind: "plain", Plain: append([]byte(nil), content...)}
}

type messageSessionAccess interface {
	GetSession(contactID string) (crypto.SessionState, bool, error)
	Encrypt(contactID string, plaintext []byte) (crypto.MessageEnvelope, error)
}

func BuildWireForOutboundMessage(msg models.Message, session messageSessionAccess) (contracts.WirePayload, bool, error) {
	if msg.ContentType == "e2ee" {
		env, err := session.Encrypt(msg.ContactID, msg.Content)
		if err != nil {
			return contracts.WirePayload{}, false, err
		}
		return contracts.WirePayload{Kind: "e2ee", Envelope: env}, true, nil
	}
	_, ok, err := session.GetSession(msg.ContactID)
	if err != nil {
		return contracts.WirePayload{}, false, err
	}
	if !ok {
		return contracts.WirePayload{}, false, messagingpolicy.ErrOutboundSessionRequired
	}
	env, err := session.Encrypt(msg.ContactID, msg.Content)
	if err != nil {
		return contracts.WirePayload{}, false, err
	}
	return contracts.WirePayload{Kind: "e2ee", Envelope: env}, true, nil
}

func NewReceiptWire(messageID, status string, now time.Time) contracts.WirePayload {
	receipt := models.MessageReceipt{MessageID: messageID, Status: status, Timestamp: now.UTC()}
	return contracts.WirePayload{Kind: "receipt", Receipt: &receipt}
}

const RetryLoopTick = 1 * time.Second
const StartupRecoveryLookahead = 24 * time.Hour

func ProcessPendingMessages(
	ctx context.Context,
	pending []PendingMessage,
	buildWire func(models.Message) (contracts.WirePayload, error),
	publish func(context.Context, string, string, contracts.WirePayload) error,
	onPublishError func(PendingMessage, error),
	onPublished func(string),
) {
	for _, p := range pending {
		wire, err := buildWire(p.Message)
		if err != nil {
			if onPublishError != nil {
				onPublishError(p, err)
			}
			continue
		}
		if err := publish(ctx, p.Message.ID, p.Message.ContactID, wire); err != nil {
			if onPublishError != nil {
				onPublishError(p, err)
			}
			continue
		}
		if onPublished != nil {
			onPublished(p.Message.ID)
		}
	}
}

type composeMessageIdentityAccess interface {
	GetIdentity() models.Identity
	ActiveDeviceAuth(payload []byte) (models.Device, []byte, error)
}

func ComposeSignedPrivateMessage(messageID, recipient string, wire contracts.WirePayload, identity composeMessageIdentityAccess) (waku.PrivateMessage, error) {
	if err := messagingpolicy.ValidateWirePayload(wire); err != nil {
		return waku.PrivateMessage{}, &contracts.CategorizedError{Category: "api", Err: err}
	}
	localIdentity := identity.GetIdentity()
	authPayload, err := BuildWireAuthPayload(messageID, localIdentity.ID, recipient, wire)
	if err != nil {
		return waku.PrivateMessage{}, &contracts.CategorizedError{Category: "api", Err: err}
	}
	device, deviceSig, err := identity.ActiveDeviceAuth(authPayload)
	if err != nil {
		return waku.PrivateMessage{}, &contracts.CategorizedError{Category: "crypto", Err: err}
	}
	wire.Device = &device
	wire.DeviceSig = append([]byte(nil), deviceSig...)
	payloadBytes, err := json.Marshal(wire)
	if err != nil {
		return waku.PrivateMessage{}, &contracts.CategorizedError{Category: "api", Err: err}
	}
	return waku.PrivateMessage{ID: messageID, SenderID: localIdentity.ID, Recipient: recipient, Payload: payloadBytes}, nil
}

func ErrorCategory(err error) string {
	var classified *contracts.CategorizedError
	if errors.As(err, &classified) {
		return classified.Category
	}
	return "api"
}

func NextRetryTime(retryCount int) time.Time {
	if retryCount < 1 {
		retryCount = 1
	}
	backoff := 2 * time.Second
	for i := 1; i < retryCount; i++ {
		backoff *= 2
		if backoff >= 30*time.Second {
			backoff = 30 * time.Second
			break
		}
	}
	return time.Now().Add(backoff)
}

type RevocationFailure struct {
	ContactID string
	Category  string
	Err       error
}

func BuildDeviceRevocationPayload(rev models.DeviceRevocation) ([]byte, error) {
	wire := contracts.WirePayload{Kind: "device_revoke", Revocation: &rev}
	return json.Marshal(wire)
}

func DispatchDeviceRevocation(localIdentityID string, contacts []models.Contact, payload []byte, nextID func() (string, error), publish func(msg waku.PrivateMessage) error) []RevocationFailure {
	failures := make([]RevocationFailure, 0)
	for _, c := range contacts {
		msgID, err := nextID()
		if err != nil {
			failures = append(failures, RevocationFailure{ContactID: c.ID, Category: "api", Err: err})
			continue
		}
		msg := waku.PrivateMessage{ID: msgID, SenderID: localIdentityID, Recipient: c.ID, Payload: payload}
		if err := publish(msg); err != nil {
			failures = append(failures, RevocationFailure{ContactID: c.ID, Category: "network", Err: err})
		}
	}
	return failures
}

func BuildDeviceRevocationDeliveryError(attempted int, failures []RevocationFailure) *contracts.DeviceRevocationDeliveryError {
	if len(failures) == 0 {
		return nil
	}
	byContact := make(map[string]string, len(failures))
	for _, f := range failures {
		byContact[f.ContactID] = f.Err.Error()
	}
	return &contracts.DeviceRevocationDeliveryError{Attempted: attempted, Failed: len(byContact), Failures: byContact}
}

func MapSessionState(state crypto.SessionState) models.SessionState {
	return models.SessionState{SessionID: state.SessionID, ContactID: state.ContactID, PeerPublicKey: append([]byte(nil), state.PeerPublicKey...), SendChainIndex: state.SendChainIndex, RecvChainIndex: state.RecvChainIndex, CreatedAt: state.CreatedAt, UpdatedAt: state.UpdatedAt}
}

func BuildWireAuthPayload(messageID, senderID, recipient string, wire contracts.WirePayload) ([]byte, error) {
	if err := messagingpolicy.ValidateWirePayload(wire); err != nil {
		return nil, err
	}
	auth := struct {
		MessageID         string `json:"message_id"`
		SenderID          string `json:"sender_id"`
		Recipient         string `json:"recipient"`
		Kind              string `json:"kind"`
		ConversationID    string `json:"conversation_id,omitempty"`
		ConversationType  string `json:"conversation_type,omitempty"`
		ThreadID          string `json:"thread_id,omitempty"`
		EventID           string `json:"event_id,omitempty"`
		EventType         string `json:"event_type,omitempty"`
		MembershipVersion uint64 `json:"membership_version,omitempty"`
		GroupKeyVersion   uint32 `json:"group_key_version,omitempty"`
		SenderDeviceID    string `json:"sender_device_id,omitempty"`
		Envelope          any    `json:"envelope"`
		Plain             []byte `json:"plain"`
		Card              any    `json:"card,omitempty"`
		Receipt           any    `json:"receipt,omitempty"`
		Revocation        any    `json:"revocation,omitempty"`
	}{
		MessageID:         messageID,
		SenderID:          senderID,
		Recipient:         recipient,
		Kind:              wire.Kind,
		ConversationID:    strings.TrimSpace(wire.ConversationID),
		ConversationType:  strings.TrimSpace(wire.ConversationType),
		ThreadID:          strings.TrimSpace(wire.ThreadID),
		EventID:           strings.TrimSpace(wire.EventID),
		EventType:         strings.TrimSpace(wire.EventType),
		MembershipVersion: wire.MembershipVersion,
		GroupKeyVersion:   wire.GroupKeyVersion,
		SenderDeviceID:    strings.TrimSpace(wire.SenderDeviceID),
		Envelope:          wire.Envelope,
		Plain:             append([]byte(nil), wire.Plain...),
		Card:              wire.Card,
		Receipt:           wire.Receipt,
		Revocation:        wire.Revocation,
	}
	return json.Marshal(auth)
}
