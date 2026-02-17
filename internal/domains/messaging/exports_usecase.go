//goland:noinspection GoNameStartsWithPackageName
package messaging

import (
	"context"
	"time"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/domains/contracts"
	messagingusecase "aim-chat/go-backend/internal/domains/messaging/usecase"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

type InboundContactTrustViolation = messagingusecase.InboundContactTrustViolation
type InboundReceiptHandling = messagingusecase.InboundReceiptHandling
type RevocationFailure = messagingusecase.RevocationFailure
type InboundPolicyAction = messagingusecase.InboundPolicyAction
type InboundPolicyDecision = messagingusecase.InboundPolicyDecision
type InboundServiceDeps = messagingusecase.InboundServiceDeps
type InboundService = messagingusecase.InboundService

const (
	RetryLoopTick             = messagingusecase.RetryLoopTick
	StartupRecoveryLookahead  = messagingusecase.StartupRecoveryLookahead
	InboundPolicyActionReject = messagingusecase.InboundPolicyActionReject
	InboundPolicyActionAccept = messagingusecase.InboundPolicyActionAccept
	InboundPolicyActionQueue  = messagingusecase.InboundPolicyActionQueue
)

func NewInboundService(deps InboundServiceDeps) *InboundService {
	return messagingusecase.NewInboundService(deps)
}

func ValidateInboundContactTrust(senderID string, wire contracts.WirePayload, identity interface {
	HasVerifiedContact(contactID string) bool
	VerifyContactCard(card models.ContactCard) (bool, error)
	ContactPublicKey(contactID string) ([]byte, bool)
	AddContact(card models.ContactCard) error
}) *InboundContactTrustViolation {
	return messagingusecase.ValidateInboundContactTrust(senderID, wire, identity)
}

func ValidateInboundDeviceAuth(msg waku.PrivateMessage, wire contracts.WirePayload, identity interface {
	VerifyInboundDevice(contactID string, device models.Device, payload, sig []byte) error
}) error {
	return messagingusecase.ValidateInboundDeviceAuth(msg, wire, identity)
}

func ResolveInboundContent(msg waku.PrivateMessage, wire contracts.WirePayload, sessions interface {
	Decrypt(contactID string, env crypto.MessageEnvelope) ([]byte, error)
}) (content []byte, contentType string, decryptErr error) {
	return messagingusecase.ResolveInboundContent(msg, wire, sessions)
}

func BuildInboundStoredMessage(msg waku.PrivateMessage, content []byte, contentType string, now time.Time) models.Message {
	return messagingusecase.BuildInboundStoredMessage(msg, content, contentType, now)
}

func BuildInboundGroupStoredMessage(msg waku.PrivateMessage, conversationID string, content []byte, contentType string, now time.Time) models.Message {
	return messagingusecase.BuildInboundGroupStoredMessage(msg, conversationID, content, contentType, now)
}

func ResolveInboundReceiptHandling(wire contracts.WirePayload) InboundReceiptHandling {
	return messagingusecase.ResolveInboundReceiptHandling(wire)
}

func AllocateOutboundMessage(contactID, content string, now func() time.Time, nextID func() (string, error), save func(models.Message) error) (models.Message, error) {
	return messagingusecase.AllocateOutboundMessage(contactID, content, now, nextID, save)
}

func NewPlainWire(content []byte) contracts.WirePayload {
	return messagingusecase.NewPlainWire(content)
}

func BuildWireForOutboundMessage(msg models.Message, session interface {
	GetSession(contactID string) (crypto.SessionState, bool, error)
	Encrypt(contactID string, plaintext []byte) (crypto.MessageEnvelope, error)
}) (contracts.WirePayload, bool, error) {
	return messagingusecase.BuildWireForOutboundMessage(msg, session)
}

func NewReceiptWire(messageID, status string, now time.Time) contracts.WirePayload {
	return messagingusecase.NewReceiptWire(messageID, status, now)
}

func ProcessPendingMessages(ctx context.Context, pending []storage.PendingMessage, buildWire func(models.Message) (contracts.WirePayload, error), publish func(context.Context, string, string, contracts.WirePayload) error, onPublishError func(storage.PendingMessage, error), onPublished func(string)) {
	messagingusecase.ProcessPendingMessages(ctx, pending, buildWire, publish, onPublishError, onPublished)
}

func ComposeSignedPrivateMessage(messageID, recipient string, wire contracts.WirePayload, identity interface {
	GetIdentity() models.Identity
	ActiveDeviceAuth(payload []byte) (models.Device, []byte, error)
}) (waku.PrivateMessage, error) {
	return messagingusecase.ComposeSignedPrivateMessage(messageID, recipient, wire, identity)
}

func ErrorCategory(err error) string {
	return messagingusecase.ErrorCategory(err)
}

func NextRetryTime(retryCount int) time.Time {
	return messagingusecase.NextRetryTime(retryCount)
}

func DispatchDeviceRevocation(localIdentityID string, contacts []models.Contact, payload []byte, nextID func() (string, error), publish func(msg waku.PrivateMessage) error) []RevocationFailure {
	return messagingusecase.DispatchDeviceRevocation(localIdentityID, contacts, payload, nextID, publish)
}

func BuildDeviceRevocationDeliveryError(attempted int, failures []RevocationFailure) *contracts.DeviceRevocationDeliveryError {
	return messagingusecase.BuildDeviceRevocationDeliveryError(attempted, failures)
}

func MapSessionState(state crypto.SessionState) models.SessionState {
	return messagingusecase.MapSessionState(state)
}

func BuildWireAuthPayload(messageID, senderID, recipient string, wire contracts.WirePayload) ([]byte, error) {
	return messagingusecase.BuildWireAuthPayload(messageID, senderID, recipient, wire)
}
