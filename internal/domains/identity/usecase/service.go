package usecase

import (
	"log/slog"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	"aim-chat/go-backend/pkg/models"
)

type identityStateStore interface {
	Persist(identityManager contracts.IdentityDomain) error
}

type Service struct {
	identityManager contracts.IdentityDomain
	identityState   identityStateStore
	messageStore    contracts.MessageRepository
	sessionManager  contracts.SessionDomain
	attachmentStore contracts.AttachmentRepository
	logger          *slog.Logger
}

func NewService(
	identityManager contracts.IdentityDomain,
	identityState identityStateStore,
	messageStore contracts.MessageRepository,
	sessionManager contracts.SessionDomain,
	attachmentStore contracts.AttachmentRepository,
	logger *slog.Logger,
) *Service {
	return &Service{
		identityManager: identityManager,
		identityState:   identityState,
		messageStore:    messageStore,
		sessionManager:  sessionManager,
		attachmentStore: attachmentStore,
		logger:          logger,
	}
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
	return CreateIdentity(password, s.identityManager, func() error {
		return s.identityState.Persist(s.identityManager)
	})
}

func (s *Service) ExportBackup(consentToken, passphrase string) (string, error) {
	result, err := ExportBackup(consentToken, passphrase, s.identityManager, s.messageStore, s.sessionManager)
	if err != nil {
		return "", err
	}
	if s.logger != nil {
		s.logger.Warn("backup export executed", "identity_id", result.IdentityID, "messages", result.MessageCount, "sessions", result.SessionCount)
	}
	return result.Blob, nil
}

func (s *Service) ImportIdentity(mnemonic, password string) (models.Identity, error) {
	return ImportIdentity(mnemonic, password, s.identityManager, func() error {
		return s.identityState.Persist(s.identityManager)
	})
}

func (s *Service) PutAttachment(name, mimeType, dataBase64 string) (models.AttachmentMeta, error) {
	name, mimeType, data, err := identitypolicy.DecodeAttachmentInput(name, mimeType, dataBase64)
	if err != nil {
		return models.AttachmentMeta{}, err
	}
	return s.attachmentStore.Put(name, mimeType, data)
}

func (s *Service) GetAttachment(attachmentID string) (models.AttachmentMeta, []byte, error) {
	attachmentID, err := identitypolicy.ValidateAttachmentID(attachmentID)
	if err != nil {
		return models.AttachmentMeta{}, nil, err
	}
	return s.attachmentStore.Get(attachmentID)
}
