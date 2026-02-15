package app

import (
	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func DecodeAttachmentInput(name, mimeType, dataBase64 string) (string, string, []byte, error) {
	name = strings.TrimSpace(name)
	mimeType = strings.TrimSpace(mimeType)
	dataBase64 = strings.TrimSpace(dataBase64)
	if name == "" || dataBase64 == "" {
		return "", "", nil, errors.New("attachment name and data are required")
	}
	if base64.StdEncoding.DecodedLen(len(dataBase64)) > MaxAttachmentBytes {
		return "", "", nil, errors.New("attachment exceeds maximum size")
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return "", "", nil, errors.New("invalid attachment encoding")
	}
	if len(data) > MaxAttachmentBytes {
		return "", "", nil, errors.New("attachment exceeds maximum size")
	}
	return name, mimeType, data, nil
}

func ValidateAttachmentID(attachmentID string) (string, error) {
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		return "", errors.New("attachment id is required")
	}
	return attachmentID, nil
}

type BackupExportResult struct {
	Blob         string
	IdentityID   string
	MessageCount int
	SessionCount int
}

type backupIdentityReader interface {
	GetIdentity() models.Identity
	Contacts() []models.Contact
}

type backupMessageSnapshotter interface {
	Snapshot() (map[string]models.Message, map[string]storage.PendingMessage)
}

type backupSessionSnapshotter interface {
	Snapshot() ([]crypto.SessionState, error)
}

func ExportBackup(consentToken, passphrase string, identity backupIdentityReader, messageStore backupMessageSnapshotter, sessionManager backupSessionSnapshotter) (BackupExportResult, error) {
	consentToken = strings.TrimSpace(consentToken)
	passphrase = strings.TrimSpace(passphrase)
	if consentToken != "I_UNDERSTAND_BACKUP_RISK" {
		return BackupExportResult{}, errors.New("backup export requires explicit consent token")
	}
	if passphrase == "" {
		return BackupExportResult{}, errors.New("backup passphrase is required")
	}

	messages, pending := messageStore.Snapshot()
	sessions, err := sessionManager.Snapshot()
	if err != nil {
		return BackupExportResult{}, err
	}
	payload := struct {
		Version    int                               `json:"version"`
		ExportedAt time.Time                         `json:"exported_at"`
		Identity   models.Identity                   `json:"identity"`
		Contacts   []models.Contact                  `json:"contacts"`
		Messages   map[string]models.Message         `json:"messages"`
		Pending    map[string]storage.PendingMessage `json:"pending"`
		Sessions   []crypto.SessionState             `json:"sessions"`
	}{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Identity:   identity.GetIdentity(),
		Contacts:   identity.Contacts(),
		Messages:   messages,
		Pending:    pending,
		Sessions:   sessions,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return BackupExportResult{}, err
	}
	encrypted, err := securestore.Encrypt(passphrase, raw)
	if err != nil {
		return BackupExportResult{}, err
	}
	return BackupExportResult{
		Blob:         base64.StdEncoding.EncodeToString(encrypted),
		IdentityID:   payload.Identity.ID,
		MessageCount: len(messages),
		SessionCount: len(sessions),
	}, nil
}

type accountIdentityAccess interface {
	GetIdentity() models.Identity
	VerifyPassword(password string) error
}

type createAccountIdentity interface {
	CreateIdentity(password string) (models.Identity, string, error)
}

type createIdentityAccess interface {
	CreateIdentity(password string) (models.Identity, string, error)
}

type importIdentityAccess interface {
	ImportIdentity(mnemonic, password string) (models.Identity, error)
}

func CreateAccount(password string, identity createAccountIdentity) (models.Account, error) {
	created, _, err := identity.CreateIdentity(password)
	if err != nil {
		return models.Account{}, err
	}
	return models.Account{
		ID:                created.ID,
		IdentityPublicKey: created.SigningPublicKey,
	}, nil
}

func Login(accountID, password string, identity accountIdentityAccess) error {
	accountID = strings.TrimSpace(accountID)
	password = strings.TrimSpace(password)
	if accountID == "" || password == "" {
		return errors.New("account id and password are required")
	}
	current := identity.GetIdentity()
	if current.ID != accountID {
		return errors.New("account id mismatch")
	}
	return identity.VerifyPassword(password)
}

func CreateIdentity(password string, identity createIdentityAccess, persist func() error) (models.Identity, string, error) {
	created, mnemonic, err := identity.CreateIdentity(strings.TrimSpace(password))
	if err != nil {
		return models.Identity{}, "", err
	}
	if err := persist(); err != nil {
		return models.Identity{}, "", err
	}
	return created, mnemonic, nil
}

func ImportIdentity(mnemonic, password string, identity importIdentityAccess, persist func() error) (models.Identity, error) {
	created, err := identity.ImportIdentity(strings.TrimSpace(mnemonic), strings.TrimSpace(password))
	if err != nil {
		return models.Identity{}, err
	}
	if err := persist(); err != nil {
		return models.Identity{}, err
	}
	return created, nil
}

type IdentityStateStore struct {
	path   string
	secret string
}

func (p *IdentityStateStore) Configure(path, secret string) {
	p.path = strings.TrimSpace(path)
	p.secret = strings.TrimSpace(secret)
}

func (p *IdentityStateStore) Bootstrap(identityManager IdentityDomain) error {
	if strings.TrimSpace(p.path) == "" || strings.TrimSpace(p.secret) == "" {
		return nil
	}
	raw, err := os.ReadFile(p.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return p.Persist(identityManager)
		}
		return err
	}
	plaintext, err := securestore.Decrypt(p.secret, raw)
	if err != nil {
		return err
	}
	var state persistedIdentityState
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return err
	}
	if state.Version != 1 || len(state.SigningPrivateKey) == 0 {
		return errors.New("identity persistence payload is invalid")
	}
	return identityManager.RestoreIdentityPrivateKey(state.SigningPrivateKey)
}

func (p *IdentityStateStore) Persist(identityManager IdentityDomain) error {
	if strings.TrimSpace(p.path) == "" || strings.TrimSpace(p.secret) == "" {
		return nil
	}
	_, privateKey := identityManager.SnapshotIdentityKeys()
	state := persistedIdentityState{
		Version:           1,
		SigningPrivateKey: privateKey,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	encrypted, err := securestore.Encrypt(p.secret, payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p.path, encrypted, 0o600)
}

type persistedIdentityState struct {
	Version           int    `json:"version"`
	SigningPrivateKey []byte `json:"signing_private_key"`
}

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

func ValidateInboundContactTrust(senderID string, wire WirePayload, identity inboundContactTrustAccess) *InboundContactTrustViolation {
	if identity.HasVerifiedContact(senderID) && wire.Card != nil {
		if wire.Card.IdentityID != senderID {
			return &InboundContactTrustViolation{
				AlertCode: "contact_card_identity_mismatch",
				Err:       errors.New("contact card identity mismatch"),
			}
		}
		ok, err := identity.VerifyContactCard(*wire.Card)
		if err != nil || !ok {
			if err == nil {
				err = errors.New("contact card verification failed")
			}
			return &InboundContactTrustViolation{
				AlertCode: "contact_card_verification_failed",
				Err:       err,
			}
		}
		if pinnedKey, exists := identity.ContactPublicKey(senderID); exists && !bytes.Equal(pinnedKey, wire.Card.PublicKey) {
			return &InboundContactTrustViolation{
				AlertCode: "contact_key_pin_mismatch",
				Err:       errors.New("contact public key changed for verified contact"),
			}
		}
	}

	if !identity.HasVerifiedContact(senderID) {
		if wire.Card == nil {
			return &InboundContactTrustViolation{
				AlertCode: "unverified_sender_missing_card",
				Err:       errors.New("unverified sender did not provide contact card"),
			}
		}
		if wire.Card.IdentityID != senderID {
			return &InboundContactTrustViolation{
				AlertCode: "contact_card_identity_mismatch",
				Err:       errors.New("contact card identity mismatch"),
			}
		}
		ok, err := identity.VerifyContactCard(*wire.Card)
		if err != nil || !ok {
			if err == nil {
				err = errors.New("contact card verification failed")
			}
			return &InboundContactTrustViolation{
				AlertCode: "contact_card_verification_failed",
				Err:       err,
			}
		}
		if err := identity.AddContact(*wire.Card); err != nil {
			return &InboundContactTrustViolation{
				AlertCode: "contact_key_pin_mismatch",
				Err:       err,
			}
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

func ValidateInboundDeviceAuth(msg waku.PrivateMessage, wire WirePayload, identity inboundDeviceVerifier) error {
	authPayload, err := BuildWireAuthPayload(msg.ID, msg.SenderID, msg.Recipient, wire)
	if err != nil {
		return &CategorizedError{Category: "api", Err: err}
	}
	if wire.Device == nil || len(wire.DeviceSig) == 0 {
		return &CategorizedError{Category: "crypto", Err: errors.New("missing device authentication")}
	}
	if err := identity.VerifyInboundDevice(msg.SenderID, *wire.Device, authPayload, wire.DeviceSig); err != nil {
		return &CategorizedError{Category: "crypto", Err: err}
	}
	return nil
}

func ShouldApplyReceiptStatus(status string) bool {
	return status == "delivered" || status == "read"
}

func ResolveInboundContent(msg waku.PrivateMessage, wire WirePayload, sessions inboundSessionDecryptor) (content []byte, contentType string, decryptErr error) {
	content = append([]byte(nil), msg.Payload...)
	contentType = "text"
	switch wire.Kind {
	case "plain":
		content = append([]byte(nil), wire.Plain...)
		contentType = "text"
	case "e2ee":
		plain, err := sessions.Decrypt(msg.SenderID, wire.Envelope)
		if err != nil {
			// Keep ciphertext for forensic/debug instead of dropping message.
			return append([]byte(nil), msg.Payload...), "e2ee-unreadable", err
		}
		return plain, "e2ee", nil
	}
	return content, contentType, nil
}

func BuildInboundStoredMessage(msg waku.PrivateMessage, content []byte, contentType string, now time.Time) models.Message {
	return models.Message{
		ID:          msg.ID,
		ContactID:   msg.SenderID,
		Content:     content,
		Timestamp:   now.UTC(),
		Direction:   "in",
		Status:      "delivered",
		ContentType: contentType,
	}
}

type InboundReceiptHandling struct {
	Handled      bool
	ShouldUpdate bool
	MessageID    string
	Status       string
}

func ResolveInboundReceiptHandling(wire WirePayload) InboundReceiptHandling {
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

func ValidateEditMessageInput(contactID, messageID, content string) (string, string, string, error) {
	contactID = strings.TrimSpace(contactID)
	messageID = strings.TrimSpace(messageID)
	content = strings.TrimSpace(content)
	if contactID == "" || messageID == "" || content == "" {
		return "", "", "", errors.New("contact id, message id and content are required")
	}
	return contactID, messageID, content, nil
}

func EnsureEditableMessage(msg models.Message, found bool, contactID string) error {
	if !found {
		return errors.New("message not found")
	}
	if msg.ContactID != contactID {
		return errors.New("message does not belong to contact")
	}
	if msg.Direction != "out" {
		return errors.New("only outbound messages can be edited")
	}
	return nil
}

func ValidateListMessagesContactID(contactID string) (string, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return "", errors.New("contact id is required")
	}
	return contactID, nil
}

func ValidateMessageStatusID(messageID string) (string, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return "", errors.New("message id is required")
	}
	return messageID, nil
}

func BuildMessageStatus(msg models.Message, found bool) (models.MessageStatus, error) {
	if !found {
		return models.MessageStatus{}, errors.New("message not found")
	}
	return models.MessageStatus{
		MessageID: msg.ID,
		Status:    msg.Status,
	}, nil
}

func ValidateSendMessageInput(contactID, content string) (string, string, error) {
	contactID = strings.TrimSpace(contactID)
	content = strings.TrimSpace(content)
	if contactID == "" || content == "" {
		return "", "", errors.New("contact id and content are required")
	}
	return contactID, content, nil
}

func NewOutboundMessage(messageID, contactID, content string, now time.Time) models.Message {
	return models.Message{
		ID:          messageID,
		ContactID:   contactID,
		Content:     []byte(content),
		Timestamp:   now.UTC(),
		Direction:   "out",
		Status:      "pending",
		ContentType: "text",
	}
}

func AllocateOutboundMessage(
	contactID, content string,
	now func() time.Time,
	nextID func() (string, error),
	save func(models.Message) error,
) (models.Message, error) {
	for i := 0; i < 3; i++ {
		msgID, err := nextID()
		if err != nil {
			return models.Message{}, err
		}
		msg := NewOutboundMessage(msgID, contactID, content, now())
		if err := save(msg); err != nil {
			if errors.Is(err, storage.ErrMessageIDConflict) {
				continue
			}
			return models.Message{}, err
		}
		return msg, nil
	}
	return models.Message{}, errors.New("failed to allocate unique message id")
}

func NewPlainWire(content []byte) WirePayload {
	return WirePayload{
		Kind:  "plain",
		Plain: append([]byte(nil), content...),
	}
}

type messageSessionAccess interface {
	GetSession(contactID string) (crypto.SessionState, bool, error)
	Encrypt(contactID string, plaintext []byte) (crypto.MessageEnvelope, error)
}

func BuildWireForOutboundMessage(msg models.Message, session messageSessionAccess) (WirePayload, bool, error) {
	if msg.ContentType == "e2ee" {
		env, err := session.Encrypt(msg.ContactID, msg.Content)
		if err != nil {
			return WirePayload{}, false, err
		}
		return WirePayload{Kind: "e2ee", Envelope: env}, true, nil
	}
	_, ok, err := session.GetSession(msg.ContactID)
	if err != nil {
		return WirePayload{}, false, err
	}
	if ok {
		env, err := session.Encrypt(msg.ContactID, msg.Content)
		if err != nil {
			return WirePayload{}, false, err
		}
		return WirePayload{Kind: "e2ee", Envelope: env}, true, nil
	}
	return NewPlainWire(msg.Content), false, nil
}

func WithSelfCard(wire WirePayload, card *models.ContactCard) WirePayload {
	if card == nil {
		return wire
	}
	wire.Card = card
	return wire
}

func ShouldAutoMarkRead(msg models.Message) bool {
	return msg.Direction == "in" && msg.Status != "read"
}

func NewReceiptWire(messageID, status string, now time.Time) WirePayload {
	receipt := models.MessageReceipt{
		MessageID: messageID,
		Status:    status,
		Timestamp: now.UTC(),
	}
	return WirePayload{
		Kind:    "receipt",
		Receipt: &receipt,
	}
}

const RetryLoopTick = 1 * time.Second
const StartupRecoveryLookahead = 24 * time.Hour

func ProcessPendingMessages(
	ctx context.Context,
	pending []storage.PendingMessage,
	buildWire func(models.Message) (WirePayload, error),
	publish func(context.Context, string, string, WirePayload) error,
	onPublishError func(storage.PendingMessage, error),
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

func ComposeSignedPrivateMessage(messageID, recipient string, wire WirePayload, identity IdentityDomain) (waku.PrivateMessage, error) {
	localIdentity := identity.GetIdentity()
	authPayload, err := BuildWireAuthPayload(messageID, localIdentity.ID, recipient, wire)
	if err != nil {
		return waku.PrivateMessage{}, &CategorizedError{Category: "api", Err: err}
	}
	device, deviceSig, err := identity.ActiveDeviceAuth(authPayload)
	if err != nil {
		return waku.PrivateMessage{}, &CategorizedError{Category: "crypto", Err: err}
	}
	wire.Device = &device
	wire.DeviceSig = append([]byte(nil), deviceSig...)

	payloadBytes, err := json.Marshal(wire)
	if err != nil {
		return waku.PrivateMessage{}, &CategorizedError{Category: "api", Err: err}
	}
	return waku.PrivateMessage{
		ID:        messageID,
		SenderID:  localIdentity.ID,
		Recipient: recipient,
		Payload:   payloadBytes,
	}, nil
}

func ErrorCategory(err error) string {
	var classified *CategorizedError
	if errors.As(err, &classified) {
		return classified.Category
	}
	return "api"
}

func NextRetryTime(retryCount int) time.Time {
	if retryCount < 1 {
		retryCount = 1
	}
	// 2s, 4s, 8s ... capped to 30s.
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

func NormalizeDeviceID(deviceID string) string {
	return strings.TrimSpace(deviceID)
}

func BuildDeviceRevocationPayload(rev models.DeviceRevocation) ([]byte, error) {
	wire := WirePayload{
		Kind:       "device_revoke",
		Revocation: &rev,
	}
	return json.Marshal(wire)
}

func DispatchDeviceRevocation(localIdentityID string, contacts []models.Contact, payload []byte, nextID func() (string, error), publish func(msg waku.PrivateMessage) error) []RevocationFailure {
	failures := make([]RevocationFailure, 0)
	for _, c := range contacts {
		msgID, err := nextID()
		if err != nil {
			failures = append(failures, RevocationFailure{
				ContactID: c.ID,
				Category:  "api",
				Err:       err,
			})
			continue
		}
		msg := waku.PrivateMessage{
			ID:        msgID,
			SenderID:  localIdentityID,
			Recipient: c.ID,
			Payload:   payload,
		}
		if err := publish(msg); err != nil {
			failures = append(failures, RevocationFailure{
				ContactID: c.ID,
				Category:  "network",
				Err:       err,
			})
		}
	}
	return failures
}

func BuildDeviceRevocationDeliveryError(attempted int, failures []RevocationFailure) *DeviceRevocationDeliveryError {
	if len(failures) == 0 {
		return nil
	}
	byContact := make(map[string]string, len(failures))
	for _, f := range failures {
		byContact[f.ContactID] = f.Err.Error()
	}
	return &DeviceRevocationDeliveryError{
		Attempted: attempted,
		Failed:    len(byContact),
		Failures:  byContact,
	}
}

func NormalizeSessionContactID(contactID string) string {
	return strings.TrimSpace(contactID)
}

func EnsureVerifiedContact(verified bool) error {
	if !verified {
		return errors.New("contact is not verified")
	}
	return nil
}

func MapSessionState(state crypto.SessionState) models.SessionState {
	return models.SessionState{
		SessionID:      state.SessionID,
		ContactID:      state.ContactID,
		PeerPublicKey:  append([]byte(nil), state.PeerPublicKey...),
		SendChainIndex: state.SendChainIndex,
		RecvChainIndex: state.RecvChainIndex,
		CreatedAt:      state.CreatedAt,
		UpdatedAt:      state.UpdatedAt,
	}
}

func BuildWireAuthPayload(messageID, senderID, recipient string, wire WirePayload) ([]byte, error) {
	auth := struct {
		MessageID  string `json:"message_id"`
		SenderID   string `json:"sender_id"`
		Recipient  string `json:"recipient"`
		Kind       string `json:"kind"`
		Envelope   any    `json:"envelope"`
		Plain      []byte `json:"plain"`
		Card       any    `json:"card,omitempty"`
		Receipt    any    `json:"receipt,omitempty"`
		Revocation any    `json:"revocation,omitempty"`
	}{
		MessageID:  messageID,
		SenderID:   senderID,
		Recipient:  recipient,
		Kind:       wire.Kind,
		Envelope:   wire.Envelope,
		Plain:      append([]byte(nil), wire.Plain...),
		Card:       wire.Card,
		Receipt:    wire.Receipt,
		Revocation: wire.Revocation,
	}
	return json.Marshal(auth)
}
