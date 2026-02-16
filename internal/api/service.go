package api

import (
	"aim-chat/go-backend/internal/app"
	"aim-chat/go-backend/internal/app/contracts"
	daemoncomposition "aim-chat/go-backend/internal/composition/daemon"
	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/identity"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type Service struct {
	identityManager   contracts.IdentityDomain
	wakuNode          contracts.TransportNode
	sessionManager    contracts.SessionDomain
	messageStore      contracts.MessageRepository
	attachmentStore   contracts.AttachmentRepository
	notifier          contracts.NotificationBus
	logger            *slog.Logger
	metrics           *app.ServiceMetricsState
	runtime           *app.ServiceRuntime
	identityState     *app.IdentityStateStore
	privacyState      *app.PrivacySettingsStateStore
	privacySettings   app.PrivacySettings
	privacyMu         sync.RWMutex
	blocklistState    *app.BlocklistStateStore
	blocklist         app.Blocklist
	blocklistMu       sync.RWMutex
	requestInboxState *app.MessageRequestStateStore
	requestInbox      map[string][]models.Message
	requestInboxMu    sync.RWMutex
	startStopMu       sync.Mutex
}

func NewService() (*Service, error) {
	return NewServiceWithConfig(waku.DefaultConfig())
}

func NewServiceWithConfig(wakuCfg waku.Config) (*Service, error) {
	return newServiceWithOptions(wakuCfg, contracts.ServiceOptions{
		SessionStore: crypto.NewInMemorySessionStore(),
		MessageStore: storage.NewMessageStore(),
		Logger:       app.DefaultLogger(),
	})
}

// noinspection GoUnusedExportedFunction
func NewServiceForDaemon(wakuCfg waku.Config) (*Service, error) {
	return NewServiceForDaemonWithDataDir(wakuCfg, "")
}

func NewServiceForDaemonWithDataDir(wakuCfg waku.Config, dataDir string) (*Service, error) {
	_, secret, bundle, err := daemoncomposition.ResolveStorage(dataDir)
	if err != nil {
		return nil, err
	}
	return newServiceForDaemonWithBundle(wakuCfg, bundle, secret)
}

func newServiceWithOptions(wakuCfg waku.Config, opts contracts.ServiceOptions) (*Service, error) {
	manager, err := identity.NewManager()
	if err != nil {
		return nil, err
	}
	if opts.SessionStore == nil {
		opts.SessionStore = crypto.NewInMemorySessionStore()
	}
	if opts.MessageStore == nil {
		opts.MessageStore = storage.NewMessageStore()
	}
	if opts.Logger == nil {
		opts.Logger = app.DefaultLogger()
	}
	if opts.AttachmentStore == nil {
		opts.AttachmentStore, err = storage.NewAttachmentStore("")
		if err != nil {
			return nil, err
		}
	}
	return &Service{
		identityManager:   manager,
		wakuNode:          waku.NewNode(wakuCfg),
		sessionManager:    crypto.NewSessionManager(opts.SessionStore),
		messageStore:      opts.MessageStore,
		attachmentStore:   opts.AttachmentStore,
		notifier:          newNotificationHub(2048),
		logger:            opts.Logger,
		metrics:           app.NewServiceMetricsState(),
		runtime:           app.NewServiceRuntime(),
		identityState:     &app.IdentityStateStore{},
		privacyState:      &app.PrivacySettingsStateStore{},
		privacySettings:   app.DefaultPrivacySettings(),
		blocklistState:    &app.BlocklistStateStore{},
		blocklist:         app.Blocklist{},
		requestInboxState: &app.MessageRequestStateStore{},
		requestInbox:      make(map[string][]models.Message),
	}, nil
}

func newServiceForDaemonWithBundle(wakuCfg waku.Config, bundle daemoncomposition.StorageBundle, secret string) (*Service, error) {
	svc, err := newServiceWithOptions(wakuCfg, contracts.ServiceOptions{
		SessionStore:    bundle.SessionStore,
		MessageStore:    bundle.MessageStore,
		AttachmentStore: bundle.AttachmentStore,
		Logger:          app.DefaultLogger(),
	})
	if err != nil {
		return nil, err
	}
	svc.identityState.Configure(bundle.IdentityPath, secret)
	if err := svc.identityState.Bootstrap(svc.identityManager); err != nil {
		return nil, err
	}
	svc.privacyState.Configure(bundle.PrivacyPath, secret)
	settings, err := svc.privacyState.Bootstrap()
	if err != nil {
		svc.logger.Warn("privacy settings bootstrap failed, using defaults", "error", err.Error())
		settings = app.DefaultPrivacySettings()
	}
	svc.privacySettings = app.NormalizePrivacySettings(settings)
	svc.blocklistState.Configure(bundle.BlocklistPath, secret)
	list, err := svc.blocklistState.Bootstrap()
	if err != nil {
		svc.logger.Warn("blocklist bootstrap failed, using empty list", "error", err.Error())
		list, _ = app.NewBlocklist(nil)
	}
	svc.blocklist = list
	svc.requestInboxState.Configure(bundle.RequestInboxPath, secret)
	inbox, err := svc.requestInboxState.Bootstrap()
	if err != nil {
		svc.logger.Warn("message request inbox bootstrap failed, using empty list", "error", err.Error())
		inbox = map[string][]models.Message{}
	}
	svc.requestInbox = app.CloneMessageRequestInbox(inbox)
	return svc, nil
}

func (s *Service) Logout() error {
	return nil
}

func (s *Service) GetIdentity() (models.Identity, error) {
	return s.identityManager.GetIdentity(), nil
}

func (s *Service) ExportSeed(password string) (string, error) {
	return s.identityManager.ExportSeed(strings.TrimSpace(password))
}

func (s *Service) ValidateMnemonic(mnemonic string) bool {
	return s.identityManager.ValidateMnemonic(strings.TrimSpace(mnemonic))
}

func (s *Service) ChangePassword(oldPassword, newPassword string) error {
	return s.identityManager.ChangePassword(strings.TrimSpace(oldPassword), strings.TrimSpace(newPassword))
}

func (s *Service) SelfContactCard(displayName string) (models.ContactCard, error) {
	return s.identityManager.SelfContactCard(displayName)
}

func (s *Service) AddContactCard(card models.ContactCard) error {
	return s.identityManager.AddContact(card)
}

func (s *Service) VerifyContactCard(card models.ContactCard) (bool, error) {
	return s.identityManager.VerifyContactCard(card)
}

func (s *Service) AddContact(contactID, displayName string) error {
	return s.identityManager.AddContactByIdentityID(contactID, displayName)
}

func (s *Service) RemoveContact(contactID string) error {
	return s.identityManager.RemoveContact(contactID)
}

func (s *Service) GetContacts() ([]models.Contact, error) {
	return s.identityManager.Contacts(), nil
}

func (s *Service) ListDevices() ([]models.Device, error) {
	return s.identityManager.ListDevices(), nil
}

func (s *Service) AddDevice(name string) (models.Device, error) {
	return s.identityManager.AddDevice(strings.TrimSpace(name))
}

func (s *Service) CreateIdentity(password string) (models.Identity, string, error) {
	return app.CreateIdentity(password, s.identityManager, func() error {
		return s.identityState.Persist(s.identityManager)
	})
}

func (s *Service) ExportBackup(consentToken, passphrase string) (string, error) {
	result, err := app.ExportBackup(consentToken, passphrase, s.identityManager, s.messageStore, s.sessionManager)
	if err != nil {
		return "", err
	}
	s.logger.Warn("backup export executed", "identity_id", result.IdentityID, "messages", result.MessageCount, "sessions", result.SessionCount)
	return result.Blob, nil
}

func (s *Service) ImportIdentity(mnemonic, password string) (models.Identity, error) {
	return app.ImportIdentity(mnemonic, password, s.identityManager, func() error {
		return s.identityState.Persist(s.identityManager)
	})
}

func (s *Service) PutAttachment(name, mimeType, dataBase64 string) (models.AttachmentMeta, error) {
	name, mimeType, data, err := app.DecodeAttachmentInput(name, mimeType, dataBase64)
	if err != nil {
		return models.AttachmentMeta{}, err
	}
	return s.attachmentStore.Put(name, mimeType, data)
}

func (s *Service) GetAttachment(attachmentID string) (models.AttachmentMeta, []byte, error) {
	attachmentID, err := app.ValidateAttachmentID(attachmentID)
	if err != nil {
		return models.AttachmentMeta{}, nil, err
	}
	return s.attachmentStore.Get(attachmentID)
}

func (s *Service) SendMessage(contactID, content string) (msgID string, err error) {
	defer s.trackOperation("message.send", &err)()
	contactID, content, err = app.ValidateSendMessageInput(contactID, content)
	if err != nil {
		return "", err
	}
	if !s.identityManager.HasContact(contactID) {
		return "", errors.New("contact is not added")
	}

	localIdentity := s.identityManager.GetIdentity()
	msg, err := app.AllocateOutboundMessage(
		contactID,
		content,
		time.Now,
		func() (string, error) { return app.GeneratePrefixedID("msg") },
		func(msg models.Message) error {
			err := s.messageStore.SaveMessage(msg)
			if err != nil && !errors.Is(err, storage.ErrMessageIDConflict) {
				s.recordError("storage", err)
			}
			return err
		},
	)
	if err != nil {
		return "", err
	}
	msgID = msg.ID
	s.notify("notify.message.new", map[string]any{
		"contact_id": contactID,
		"message":    msg,
	})
	wire := app.NewPlainWire(msg.Content)
	if card, err := s.identityManager.SelfContactCard(localIdentity.ID); err == nil {
		wire = app.WithSelfCard(wire, &card)
	}
	builtWire, encrypted, werr := app.BuildWireForOutboundMessage(msg, s.sessionManager)
	if werr != nil {
		s.recordError("crypto", werr)
		return "", werr
	}
	if encrypted {
		wire = builtWire
		msg.ContentType = "e2ee"
	} else if builtWire.Kind == "plain" {
		// Preserve attached self-card on plain wire.
		if wire.Card != nil {
			builtWire.Card = wire.Card
		}
		wire = builtWire
	}

	return s.publishQueuedMessage(msg, contactID, wire)
}

func (s *Service) EditMessage(contactID, messageID, content string) (models.Message, error) {
	contactID, messageID, content, err := app.ValidateEditMessageInput(contactID, messageID, content)
	if err != nil {
		return models.Message{}, err
	}
	msg, ok := s.messageStore.GetMessage(messageID)
	if err := app.EnsureEditableMessage(msg, ok, contactID); err != nil {
		return models.Message{}, err
	}

	updated, ok, err := s.messageStore.UpdateMessageContent(messageID, []byte(content), msg.ContentType)
	if err != nil {
		s.recordError("storage", err)
		return models.Message{}, err
	}
	if !ok {
		return models.Message{}, errors.New("message not found")
	}

	s.notify("notify.message.status", map[string]any{
		"contact_id": contactID,
		"message_id": messageID,
		"status":     updated.Status,
		"edited":     true,
	})
	return updated, nil
}

func (s *Service) DeleteMessage(contactID, messageID string) error {
	contactID = strings.TrimSpace(contactID)
	messageID = strings.TrimSpace(messageID)
	if contactID == "" || messageID == "" {
		return errors.New("invalid params")
	}
	deleted, err := s.messageStore.DeleteMessage(contactID, messageID)
	if err != nil {
		s.recordError("storage", err)
		return err
	}
	if !deleted {
		return errors.New("message not found")
	}
	s.notify("notify.message.deleted", map[string]any{
		"contact_id": contactID,
		"message_id": messageID,
	})
	return nil
}

func (s *Service) ClearMessages(contactID string) (int, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return 0, errors.New("invalid params")
	}
	deleted, err := s.messageStore.ClearMessages(contactID)
	if err != nil {
		s.recordError("storage", err)
		return 0, err
	}
	s.notify("notify.message.cleared", map[string]any{
		"contact_id": contactID,
		"count":      deleted,
	})
	return deleted, nil
}

func (s *Service) GetMessages(contactID string, limit, offset int) (messages []models.Message, err error) {
	defer s.trackOperation("message.list", &err)()
	contactID, err = app.ValidateListMessagesContactID(contactID)
	if err != nil {
		return nil, err
	}
	messages = s.messageStore.ListMessages(contactID, limit, offset)
	for i := range messages {
		msg := messages[i]
		if !app.ShouldAutoMarkRead(msg) {
			continue
		}
		s.applyAutoRead(&messages[i], contactID)
	}
	return messages, nil
}

func (s *Service) ListMessageRequests() ([]models.MessageRequest, error) {
	s.requestInboxMu.RLock()
	defer s.requestInboxMu.RUnlock()

	out := make([]models.MessageRequest, 0, len(s.requestInbox))
	for senderID, messages := range s.requestInbox {
		summary, err := app.BuildMessageRequestSummary(senderID, messages)
		if err != nil {
			continue
		}
		out = append(out, summary)
	}
	app.SortMessageRequestsByRecency(out)
	return out, nil
}

func (s *Service) persistRequestInboxLocked() error {
	if s.requestInboxState == nil {
		return nil
	}
	return s.requestInboxState.Persist(s.requestInbox)
}

func (s *Service) GetMessageRequest(senderID string) (models.MessageRequestThread, error) {
	senderID, err := app.ValidateListMessagesContactID(senderID)
	if err != nil {
		return models.MessageRequestThread{}, err
	}

	s.requestInboxMu.RLock()
	thread, ok := s.requestInbox[senderID]
	s.requestInboxMu.RUnlock()
	if !ok || len(thread) == 0 {
		return models.MessageRequestThread{}, app.ErrMessageRequestNotFound
	}

	summary, err := app.BuildMessageRequestSummary(senderID, thread)
	if err != nil {
		return models.MessageRequestThread{}, err
	}
	return models.MessageRequestThread{
		Request:  summary,
		Messages: app.CloneMessages(thread),
	}, nil
}

func (s *Service) AcceptMessageRequest(senderID string) (bool, error) {
	senderID, err := app.ValidateListMessagesContactID(senderID)
	if err != nil {
		return false, err
	}

	s.requestInboxMu.Lock()
	thread, exists := s.requestInbox[senderID]
	if !exists || len(thread) == 0 {
		s.requestInboxMu.Unlock()
		if s.identityManager.HasContact(senderID) {
			return true, nil
		}
		return false, app.ErrMessageRequestNotFound
	}
	thread = app.CloneMessages(thread)
	delete(s.requestInbox, senderID)
	if err := s.persistRequestInboxLocked(); err != nil {
		s.requestInbox[senderID] = thread
		s.requestInboxMu.Unlock()
		s.recordError("storage", err)
		return false, err
	}
	s.requestInboxMu.Unlock()

	if err := s.AddContact(senderID, senderID); err != nil {
		// Restore request for retry if contact addition failed.
		s.requestInboxMu.Lock()
		if len(s.requestInbox[senderID]) == 0 {
			s.requestInbox[senderID] = thread
			if perr := s.persistRequestInboxLocked(); perr != nil {
				s.requestInboxMu.Unlock()
				s.recordError("storage", perr)
				return false, errors.Join(err, perr)
			}
		}
		s.requestInboxMu.Unlock()
		return false, err
	}

	for _, msg := range thread {
		if err := s.messageStore.SaveMessage(msg); err != nil {
			if errors.Is(err, storage.ErrMessageIDConflict) {
				continue
			}
			s.recordError("storage", err)
			return false, err
		}
		s.notify("notify.message.new", map[string]any{
			"contact_id": senderID,
			"message":    msg,
		})
	}
	s.notify("notify.request.accepted", map[string]any{
		"contact_id": senderID,
		"moved":      len(thread),
	})
	return true, nil
}

func (s *Service) DeclineMessageRequest(senderID string) (bool, error) {
	senderID, err := app.ValidateListMessagesContactID(senderID)
	if err != nil {
		return false, err
	}

	exists, err := s.removeMessageRequest(senderID)
	if err != nil {
		s.recordError("storage", err)
		return false, err
	}

	if exists {
		s.notify("notify.request.declined", map[string]any{
			"contact_id": senderID,
		})
	}
	// Idempotent: repeated decline is treated as success.
	return true, nil
}

func (s *Service) BlockSender(senderID string) (models.BlockSenderResult, error) {
	senderID, err := app.NormalizeIdentityID(senderID)
	if err != nil {
		return models.BlockSenderResult{}, err
	}
	contactExists := s.identityManager.HasContact(senderID)
	blocked, err := s.AddToBlocklist(senderID)
	if err != nil {
		return models.BlockSenderResult{}, err
	}
	requestRemoved, err := s.removeMessageRequest(senderID)
	if err != nil {
		s.recordError("storage", err)
		return models.BlockSenderResult{}, err
	}
	if requestRemoved {
		s.notify("notify.request.blocked", map[string]any{
			"contact_id": senderID,
		})
	}
	s.notify("notify.sender.blocked", map[string]any{
		"contact_id":      senderID,
		"request_removed": requestRemoved,
		"contact_exists":  contactExists,
	})
	return models.BlockSenderResult{
		Blocked:        blocked,
		RequestRemoved: requestRemoved,
		ContactExists:  contactExists,
	}, nil
}

func (s *Service) removeMessageRequest(senderID string) (bool, error) {
	s.requestInboxMu.Lock()
	defer s.requestInboxMu.Unlock()
	if _, exists := s.requestInbox[senderID]; !exists {
		return false, nil
	}
	delete(s.requestInbox, senderID)
	if err := s.persistRequestInboxLocked(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) GetMessageStatus(messageID string) (status models.MessageStatus, err error) {
	defer s.trackOperation("message.status", &err)()
	messageID, err = app.ValidateMessageStatusID(messageID)
	if err != nil {
		return models.MessageStatus{}, err
	}
	msg, ok := s.messageStore.GetMessage(messageID)
	return app.BuildMessageStatus(msg, ok)
}

func (s *Service) buildStoredMessageWire(msg models.Message) (app.WirePayload, error) {
	wire, _, err := app.BuildWireForOutboundMessage(msg, s.sessionManager)
	if err != nil {
		return app.WirePayload{}, &app.CategorizedError{Category: "crypto", Err: err}
	}
	return wire, nil
}

func (s *Service) sendReceipt(contactID, messageID, status string) error {
	if !s.identityManager.HasVerifiedContact(contactID) {
		return errors.New("receipt target is not a verified contact")
	}
	wire := app.NewReceiptWire(messageID, status, time.Now())
	wireID, err := app.GeneratePrefixedID("rcpt")
	if err != nil {
		return err
	}
	ctx, err := s.networkContext("network")
	if err != nil {
		return err
	}
	return s.publishSignedWireWithContext(ctx, wireID, contactID, wire)
}

func (s *Service) applyAutoRead(message *models.Message, contactID string) {
	if message == nil {
		return
	}
	if !s.updateMessageStatusAndNotify(message.ID, "read") {
		return
	}
	message.Status = "read"
	if err := s.sendReceipt(contactID, message.ID, "read"); err != nil {
		s.recordError("network", err)
	}
}

func (s *Service) publishQueuedMessage(msg models.Message, contactID string, wire app.WirePayload) (string, error) {
	s.logger.Info("message queued", "message_id", msg.ID, "contact_id", contactID, "kind", wire.Kind)
	ctx, err := s.networkContext("network")
	if err == nil {
		err = s.publishSignedWireWithContext(ctx, msg.ID, contactID, wire)
	}
	if err != nil {
		category := app.ErrorCategory(err)
		s.recordError(category, err)
		if category == "network" {
			if perr := s.messageStore.AddOrUpdatePending(msg, 1, app.NextRetryTime(1), err.Error()); perr != nil {
				s.recordError("storage", perr)
				return "", perr
			}
			return msg.ID, nil
		}
		return "", err
	}
	s.logger.Info("message published", "message_id", msg.ID, "contact_id", contactID)
	s.markMessageAsSent(msg.ID)
	return msg.ID, nil
}

func (s *Service) InitSession(contactID string, peerPublicKey []byte) (session models.SessionState, err error) {
	defer s.trackOperation("session.init", &err)()
	contactID = app.NormalizeSessionContactID(contactID)
	if err := app.EnsureVerifiedContact(s.identityManager.HasVerifiedContact(contactID)); err != nil {
		return models.SessionState{}, err
	}
	localIdentity := s.identityManager.GetIdentity()
	state, err := s.sessionManager.InitSession(localIdentity.ID, contactID, peerPublicKey)
	if err != nil {
		s.recordError("crypto", err)
		return models.SessionState{}, err
	}
	session = app.MapSessionState(state)
	return session, nil
}

func (s *Service) RevokeDevice(deviceID string) (models.DeviceRevocation, error) {
	deviceID = app.NormalizeDeviceID(deviceID)
	rev, err := s.identityManager.RevokeDevice(deviceID)
	if err != nil {
		return models.DeviceRevocation{}, err
	}
	contacts := s.identityManager.Contacts()
	payloadBytes, err := app.BuildDeviceRevocationPayload(rev)
	if err != nil {
		s.recordError("api", err)
		return models.DeviceRevocation{}, err
	}
	localIdentity := s.identityManager.GetIdentity()
	failures := app.DispatchDeviceRevocation(localIdentity.ID, contacts, payloadBytes, func() (string, error) {
		return app.GeneratePrefixedID("rev")
	}, func(msg waku.PrivateMessage) error {
		ctx, err := s.networkContext("")
		if err != nil {
			return err
		}
		return s.publishWithTimeout(ctx, msg)
	})
	for _, f := range failures {
		if f.Err != nil {
			s.recordError(f.Category, f.Err)
		}
	}
	if deliveryErr := app.BuildDeviceRevocationDeliveryError(len(contacts), failures); deliveryErr != nil {
		return rev, deliveryErr
	}
	return rev, nil
}

func (s *Service) handleIncomingPrivateMessage(msg waku.PrivateMessage) {
	decision := s.evaluateInboundPolicy(msg.SenderID)
	switch decision.Action {
	case app.InboundMessageActionReject:
		s.recordError("crypto", inboundPolicyError(decision.Reason))
		return
	case app.InboundMessageActionQueueRequest:
		s.handleInboundMessageRequest(msg)
		return
	}

	content := append([]byte(nil), msg.Payload...)
	contentType := "text"

	var wire app.WirePayload
	if err := json.Unmarshal(msg.Payload, &wire); err == nil {
		if !s.shouldBypassContactTrust(decision, wire, msg.SenderID) {
			if violation := app.ValidateInboundContactTrust(msg.SenderID, wire, s.identityManager); violation != nil {
				s.recordError("crypto", violation.Err)
				s.notifySecurityAlert(violation.AlertCode, msg.SenderID, violation.Err.Error())
				return
			}
		}

		if wire.Kind == "device_revoke" && wire.Revocation != nil {
			if err := s.identityManager.ApplyDeviceRevocation(msg.SenderID, *wire.Revocation); err != nil {
				s.recordError("crypto", err)
			}
			return
		}
		if !s.shouldBypassInboundDeviceAuth(decision, wire, msg.SenderID) {
			if err := app.ValidateInboundDeviceAuth(msg, wire, s.identityManager); err != nil {
				s.recordError(app.ErrorCategory(err), err)
				return
			}
		}
		receiptHandling := app.ResolveInboundReceiptHandling(wire)
		if receiptHandling.Handled {
			if receiptHandling.ShouldUpdate {
				s.applyInboundReceiptStatus(receiptHandling)
			}
			return
		}
		var decryptErr error
		content, contentType, decryptErr = app.ResolveInboundContent(msg, wire, s.sessionManager)
		if decryptErr != nil {
			s.recordError("crypto", decryptErr)
		}
	}

	in := app.BuildInboundStoredMessage(msg, content, contentType, time.Now())
	if !s.persistInboundMessage(in, msg.SenderID) {
		return
	}
	if !s.identityManager.HasVerifiedContact(msg.SenderID) {
		return
	}
	if err := s.sendReceipt(msg.SenderID, msg.ID, "delivered"); err != nil {
		s.recordError("network", err)
	}
}

func (s *Service) shouldBypassContactTrust(
	decision app.InboundMessagePolicyDecision,
	wire app.WirePayload,
	senderID string,
) bool {
	if decision.Reason != app.InboundMessageReasonUnknownEveryoneMode {
		return false
	}
	if s.identityManager.HasVerifiedContact(senderID) {
		return false
	}
	if wire.Card != nil {
		return false
	}
	if err := s.identityManager.AddContactByIdentityID(senderID, senderID); err != nil {
		s.recordError("crypto", err)
		return false
	}
	return true
}

func (s *Service) shouldBypassInboundDeviceAuth(
	decision app.InboundMessagePolicyDecision,
	wire app.WirePayload,
	senderID string,
) bool {
	if decision.Reason != app.InboundMessageReasonUnknownEveryoneMode {
		return false
	}
	if s.identityManager.HasVerifiedContact(senderID) {
		return false
	}
	return wire.Card == nil
}

func (s *Service) handleInboundMessageRequest(msg waku.PrivateMessage) {
	content := append([]byte(nil), msg.Payload...)
	contentType := "text"

	var wire app.WirePayload
	if err := json.Unmarshal(msg.Payload, &wire); err == nil {
		receiptHandling := app.ResolveInboundReceiptHandling(wire)
		if receiptHandling.Handled {
			return
		}
		var decryptErr error
		content, contentType, decryptErr = app.ResolveInboundContent(msg, wire, s.sessionManager)
		if decryptErr != nil {
			s.recordError("crypto", decryptErr)
		}
	}

	in := app.BuildInboundStoredMessage(msg, content, contentType, time.Now())
	if !s.persistInboundRequest(in) {
		return
	}
	if !s.identityManager.HasVerifiedContact(msg.SenderID) {
		return
	}
	if err := s.sendReceipt(msg.SenderID, msg.ID, "delivered"); err != nil {
		s.recordError("network", err)
	}
}

func (s *Service) evaluateInboundPolicy(senderID string) app.InboundMessagePolicyDecision {
	return app.EvaluateInboundMessagePolicy(app.InboundMessagePolicyInput{
		IsKnownContact: s.identityManager.HasContact(senderID),
		IsBlocked:      s.isBlockedSender(senderID),
		PrivacyMode:    s.currentPrivacyMode(),
	})
}

func (s *Service) currentPrivacyMode() app.MessagePrivacyMode {
	s.privacyMu.RLock()
	mode := s.privacySettings.MessagePrivacyMode
	s.privacyMu.RUnlock()
	return mode
}

func (s *Service) isBlockedSender(senderID string) bool {
	s.blocklistMu.RLock()
	blocked := s.blocklist.Contains(senderID)
	s.blocklistMu.RUnlock()
	return blocked
}

func inboundPolicyError(reason app.InboundMessagePolicyReason) error {
	switch reason {
	case app.InboundMessageReasonBlockedSender:
		return errors.New("sender is blocked")
	case app.InboundMessageReasonUnknownContactsOnly:
		return errors.New("sender is not an added contact")
	case app.InboundMessageReasonUnknownMessageReq:
		return errors.New("sender must be accepted through message requests")
	default:
		return errors.New("inbound message rejected by policy")
	}
}

func (s *Service) applyInboundReceiptStatus(receiptHandling app.InboundReceiptHandling) {
	s.updateMessageStatusAndNotify(receiptHandling.MessageID, receiptHandling.Status)
}

func (s *Service) persistInboundMessage(in models.Message, senderID string) bool {
	if err := s.messageStore.SaveMessage(in); err != nil {
		if errors.Is(err, storage.ErrMessageIDConflict) {
			s.logger.Warn("inbound message id conflict ignored", "message_id", in.ID, "contact_id", in.ContactID)
			return false
		}
		s.recordError("storage", err)
		return false
	}
	s.logger.Info("message received", "message_id", in.ID, "contact_id", in.ContactID, "content_type", in.ContentType)
	s.notify("notify.message.new", map[string]any{
		"contact_id": senderID,
		"message":    in,
	})
	return true
}

func (s *Service) persistInboundRequest(in models.Message) bool {
	s.requestInboxMu.Lock()

	thread := s.requestInbox[in.ContactID]
	if app.HasMessageID(thread, in.ID) {
		s.requestInboxMu.Unlock()
		s.logger.Warn("inbound request message id conflict ignored", "message_id", in.ID, "contact_id", in.ContactID)
		return false
	}
	in.Timestamp = app.NormalizeMessageTimestamp(in.Timestamp)
	s.requestInbox[in.ContactID] = append(thread, in)
	if err := s.persistRequestInboxLocked(); err != nil {
		// Best-effort rollback for append failure.
		if len(thread) == 0 {
			delete(s.requestInbox, in.ContactID)
		} else {
			s.requestInbox[in.ContactID] = thread
		}
		s.requestInboxMu.Unlock()
		s.recordError("storage", err)
		return false
	}
	s.requestInboxMu.Unlock()
	s.notify("notify.request.new", map[string]any{
		"contact_id": in.ContactID,
		"message":    in,
	})
	return true
}

func (s *Service) StartNetworking(ctx context.Context) error {
	s.startStopMu.Lock()
	defer s.startStopMu.Unlock()

	if s.runtime.IsNetworking() {
		return nil
	}

	if err := s.wakuNode.Start(ctx); err != nil {
		s.recordError("network", err)
		return err
	}
	localIdentity := s.identityManager.GetIdentity()
	s.wakuNode.SetIdentity(localIdentity.ID)
	if err := s.wakuNode.SubscribePrivate(s.handleIncomingPrivateMessage); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.wakuNode.Stop(stopCtx)
		cancel()
		s.recordError("network", err)
		return err
	}
	s.syncMissedInboundMessages(localIdentity.ID)
	networkCtx, networkCancel := context.WithCancel(ctx)
	s.recoverPendingOnStartup(networkCtx)

	retryCtx, cancel := context.WithCancel(networkCtx)
	if !s.runtime.TryActivate(networkCtx, networkCancel, cancel) {
		cancel()
		networkCancel()
		return nil
	}
	go func() {
		defer s.runtime.RetryLoopDone()
		s.runRetryLoop(retryCtx)
	}()
	s.notifyNetworkStatus(true)
	return nil
}

func (s *Service) syncMissedInboundMessages(identityID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	missed, err := s.wakuNode.FetchPrivateSince(ctx, identityID, time.Now().Add(-24*time.Hour), 500)
	if err != nil {
		s.recordError("network", err)
		return
	}
	for _, msg := range missed {
		s.handleIncomingPrivateMessage(msg)
	}
}

func (s *Service) StopNetworking(ctx context.Context) error {
	s.startStopMu.Lock()
	defer s.startStopMu.Unlock()

	retryCancel, networkCancel, wasRunning := s.runtime.Deactivate()
	if !wasRunning {
		return nil
	}

	if retryCancel != nil {
		retryCancel()
		s.runtime.WaitRetryLoop()
	}
	if networkCancel != nil {
		networkCancel()
	}
	if err := s.wakuNode.Stop(ctx); err != nil {
		s.recordError("network", err)
		return err
	}
	s.notifyNetworkStatus(true)
	return nil
}

func (s *Service) runRetryLoop(ctx context.Context) {
	ticker := time.NewTicker(app.RetryLoopTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.notifyNetworkStatus(false)
			now := time.Now()
			pending := s.messageStore.DuePending(now)
			s.processPendingBatch(ctx, pending, s.handleRetryPublishError)
		}
	}
}

func (s *Service) notifyNetworkStatus(force bool) {
	current := s.GetNetworkStatus()
	shouldNotify := s.runtime.UpdateLastNetworkStatus(current, force)
	if shouldNotify {
		s.notify("notify.network", current)
	}
}

func (s *Service) recoverPendingOnStartup(ctx context.Context) {
	pending := s.messageStore.DuePending(time.Now().Add(app.StartupRecoveryLookahead))
	if len(pending) == 0 {
		return
	}
	s.logger.Info("startup recovery", "pending_count", len(pending))
	s.processPendingBatch(ctx, pending, s.handleStartupPublishError)
}

func (s *Service) publishSignedWireWithContext(ctx context.Context, messageID, recipient string, wire app.WirePayload) error {
	wmsg, err := app.ComposeSignedPrivateMessage(messageID, recipient, wire, s.identityManager)
	if err != nil {
		return err
	}
	if err := s.publishWithTimeout(ctx, wmsg); err != nil {
		return &app.CategorizedError{Category: "network", Err: err}
	}
	return nil
}

func (s *Service) markMessageAsSent(messageID string) {
	s.updateMessageStatusAndNotify(messageID, "sent")
	if err := s.messageStore.RemovePending(messageID); err != nil {
		s.recordError("storage", err)
	}
}

func (s *Service) publishWithTimeout(parent context.Context, msg waku.PrivateMessage) error {
	if parent == nil {
		return errors.New("publish context is not available")
	}
	publishCtx, cancel := context.WithTimeout(parent, app.PublishTimeout)
	defer cancel()
	return s.wakuNode.PublishPrivate(publishCtx, msg)
}

func (s *Service) networkContext(category string) (context.Context, error) {
	ctx, ok := s.runtime.CurrentNetworkContext()
	if ok {
		return ctx, nil
	}
	err := errors.New("networking is not started")
	if category == "" {
		return nil, err
	}
	return nil, &app.CategorizedError{Category: category, Err: err}
}

func (s *Service) GetNetworkStatus() models.NetworkStatus {
	status := s.wakuNode.Status()
	return models.NetworkStatus{
		Status:    status.State,
		PeerCount: status.PeerCount,
		LastSync:  status.LastSync,
	}
}

func (s *Service) ListenAddresses() []string {
	return s.wakuNode.ListenAddresses()
}

func (s *Service) processPendingBatch(
	ctx context.Context,
	pending []storage.PendingMessage,
	onPublishError func(storage.PendingMessage, error),
) {
	app.ProcessPendingMessages(
		ctx,
		pending,
		func(msg models.Message) (app.WirePayload, error) {
			return s.buildStoredMessageWire(msg)
		},
		func(parent context.Context, messageID, recipient string, wire app.WirePayload) error {
			return s.publishSignedWireWithContext(parent, messageID, recipient, wire)
		},
		onPublishError,
		func(messageID string) {
			s.markMessageAsSent(messageID)
		},
	)
}

func (s *Service) handleRetryPublishError(p storage.PendingMessage, err error) {
	s.recordError(app.ErrorCategory(err), err)
	nextCount := p.RetryCount + 1
	s.recordRetryAttempt()
	s.logger.Warn("message retry scheduled", "message_id", p.Message.ID, "contact_id", p.Message.ContactID, "retry_count", nextCount)
	if perr := s.messageStore.AddOrUpdatePending(p.Message, nextCount, app.NextRetryTime(nextCount), err.Error()); perr != nil {
		s.recordError("storage", perr)
	}
}

func (s *Service) handleStartupPublishError(_ storage.PendingMessage, err error) {
	s.recordError(app.ErrorCategory(err), err)
}

func (s *Service) SubscribeNotifications(cursor int64) ([]NotificationEvent, <-chan NotificationEvent, func()) {
	return s.notifier.Subscribe(cursor)
}

func (s *Service) notify(method string, payload any) {
	s.notifier.Publish(method, payload)
}

func (s *Service) notifyMessageStatus(messageID, status string) {
	msg, ok := s.messageStore.GetMessage(messageID)
	if !ok {
		return
	}
	s.notify("notify.message.status", map[string]any{
		"message_id": messageID,
		"contact_id": msg.ContactID,
		"status":     status,
	})
}

func (s *Service) notifySecurityAlert(kind, contactID, message string) {
	s.notify("notify.security.alert", map[string]any{
		"kind":       kind,
		"contact_id": contactID,
		"message":    message,
	})
}

func (s *Service) updateMessageStatusAndNotify(messageID, status string) bool {
	if _, err := s.messageStore.UpdateMessageStatus(messageID, status); err != nil {
		s.recordError("storage", err)
		return false
	}
	s.notifyMessageStatus(messageID, status)
	return true
}

func (s *Service) GetMetrics() models.MetricsSnapshot {
	status := s.wakuNode.Status()
	counters, opStats, retries, lastAt := s.metrics.Snapshot()
	return models.MetricsSnapshot{
		PeerCount:           status.PeerCount,
		PendingQueueSize:    s.messageStore.PendingCount(),
		ErrorCounters:       counters,
		NetworkMetrics:      s.wakuNode.NetworkMetrics(),
		OperationStats:      opStats,
		RetryAttemptsTotal:  retries,
		LastUpdatedAt:       lastAt,
		NotificationBacklog: s.notifier.BacklogSize(),
	}
}

func (s *Service) GetPrivacySettings() (app.PrivacySettings, error) {
	s.privacyMu.RLock()
	settings := s.privacySettings
	s.privacyMu.RUnlock()
	return app.NormalizePrivacySettings(settings), nil
}

func (s *Service) UpdatePrivacySettings(mode string) (app.PrivacySettings, error) {
	parsedMode, err := app.ParseMessagePrivacyMode(mode)
	if err != nil {
		return app.PrivacySettings{}, err
	}
	updated := app.PrivacySettings{MessagePrivacyMode: parsedMode}
	updated = app.NormalizePrivacySettings(updated)
	if err := s.privacyState.Persist(updated); err != nil {
		s.recordError("storage", err)
		return app.PrivacySettings{}, err
	}
	s.privacyMu.Lock()
	s.privacySettings = updated
	s.privacyMu.Unlock()
	return updated, nil
}

func (s *Service) GetBlocklist() ([]string, error) {
	s.blocklistMu.RLock()
	out := s.blocklist.List()
	s.blocklistMu.RUnlock()
	return out, nil
}

func (s *Service) AddToBlocklist(identityID string) ([]string, error) {
	s.blocklistMu.Lock()
	defer s.blocklistMu.Unlock()

	next, err := app.NewBlocklist(s.blocklist.List())
	if err != nil {
		return nil, err
	}
	if err := next.Add(identityID); err != nil {
		return nil, err
	}
	if err := s.blocklistState.Persist(next); err != nil {
		s.recordError("storage", err)
		return nil, err
	}
	s.blocklist = next
	return next.List(), nil
}

func (s *Service) RemoveFromBlocklist(identityID string) ([]string, error) {
	s.blocklistMu.Lock()
	defer s.blocklistMu.Unlock()

	next, err := app.NewBlocklist(s.blocklist.List())
	if err != nil {
		return nil, err
	}
	if err := next.Remove(identityID); err != nil {
		return nil, err
	}
	if err := s.blocklistState.Persist(next); err != nil {
		s.recordError("storage", err)
		return nil, err
	}
	s.blocklist = next
	return next.List(), nil
}

func (s *Service) recordError(category string, err error) {
	s.metrics.RecordError(category)
	s.logger.Error("service error", "category", category, "error", err.Error())
}

func (s *Service) recordRetryAttempt() {
	s.metrics.RecordRetryAttempt()
}

func (s *Service) recordOp(operation string, started time.Time) {
	s.metrics.RecordOp(operation, started)
}

func (s *Service) recordOpError(operation string) {
	s.metrics.RecordOpError(operation)
}

func (s *Service) trackOperation(operation string, errRef *error) func() {
	started := time.Now()
	return func() {
		s.recordOp(operation, started)
		if errRef != nil && *errRef != nil {
			s.recordOpError(operation)
		}
	}
}
