package usecase

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"aim-chat/go-backend/internal/domains/contracts"
	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	identityports "aim-chat/go-backend/internal/domains/identity/ports"
	"aim-chat/go-backend/pkg/models"
)

type Service struct {
	identityManager contracts.IdentityDomain
	identityState   identityports.IdentityStateStore
	messageStore    contracts.MessageRepository
	sessionManager  contracts.SessionDomain
	attachmentStore contracts.AttachmentRepository
	logger          *slog.Logger
	uploadMu        sync.Mutex
	uploads         map[string]attachmentUploadSession
}

func NewService(
	identityManager contracts.IdentityDomain,
	identityState identityports.IdentityStateStore,
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
		uploads:         make(map[string]attachmentUploadSession),
	}
}

func (s *Service) Logout() error {
	return nil
}

func (s *Service) GetIdentity() (models.Identity, error) {
	return s.identityManager.GetIdentity(), nil
}

func (s *Service) Login(identityID, seedPassword string) error {
	return Login(strings.TrimSpace(identityID), strings.TrimSpace(seedPassword), s.identityManager)
}

func (s *Service) ExportSeed(seedPassword string) (string, error) {
	return s.identityManager.ExportSeed(strings.TrimSpace(seedPassword))
}

func (s *Service) ValidateMnemonic(mnemonic string) bool {
	return s.identityManager.ValidateMnemonic(strings.TrimSpace(mnemonic))
}

func (s *Service) ChangePassword(oldSeedPassword, newSeedPassword string) error {
	return s.identityManager.ChangePassword(strings.TrimSpace(oldSeedPassword), strings.TrimSpace(newSeedPassword))
}

func (s *Service) SelfContactCard(displayName string) (models.ContactCard, error) {
	return s.identityManager.SelfContactCard(displayName)
}

func (s *Service) AddContactCard(card models.ContactCard) error {
	if err := s.identityManager.AddContact(card); err != nil {
		return err
	}
	return s.identityState.Persist(s.identityManager)
}

func (s *Service) VerifyContactCard(card models.ContactCard) (bool, error) {
	return s.identityManager.VerifyContactCard(card)
}

func (s *Service) AddContact(contactID, displayName string) error {
	if err := s.identityManager.AddContactByIdentityID(contactID, displayName); err != nil {
		return err
	}
	return s.identityState.Persist(s.identityManager)
}

func (s *Service) RemoveContact(contactID string) error {
	if err := s.identityManager.RemoveContact(contactID); err != nil {
		return err
	}
	return s.identityState.Persist(s.identityManager)
}

func (s *Service) GetContacts() ([]models.Contact, error) {
	return s.identityManager.Contacts(), nil
}

func (s *Service) ListDevices() ([]models.Device, error) {
	return s.identityManager.ListDevices(), nil
}

func (s *Service) AddDevice(name string) (models.Device, error) {
	device, err := s.identityManager.AddDevice(strings.TrimSpace(name))
	if err != nil {
		return models.Device{}, err
	}
	if err := s.identityState.Persist(s.identityManager); err != nil {
		return models.Device{}, err
	}
	return device, nil
}

func (s *Service) CreateIdentity(seedPassword string) (models.Identity, string, error) {
	return CreateIdentity(seedPassword, s.identityManager, func() error {
		return s.identityState.Persist(s.identityManager)
	})
}

func (s *Service) ExportBackup(consentToken, password string) (string, error) {
	result, err := ExportBackup(consentToken, password, s.identityManager, s.messageStore, s.sessionManager)
	if err != nil {
		return "", err
	}
	if s.logger != nil {
		s.logger.Warn("backup export executed", "identity_id", result.IdentityID, "messages", result.MessageCount, "sessions", result.SessionCount)
	}
	return result.Blob, nil
}

func (s *Service) RestoreBackup(consentToken, password, backupBlob string) (models.Identity, error) {
	result, err := RestoreBackup(consentToken, password, backupBlob, s.identityManager, s.messageStore, s.sessionManager)
	if err != nil {
		return models.Identity{}, err
	}
	if err := s.identityState.Persist(s.identityManager); err != nil {
		return models.Identity{}, err
	}
	if s.logger != nil {
		s.logger.Warn("backup restore executed", "identity_id", result.IdentityID, "messages", result.MessageCount, "sessions", result.SessionCount)
	}
	return s.identityManager.GetIdentity(), nil
}

func (s *Service) ImportIdentity(mnemonic, seedPassword string) (models.Identity, error) {
	return ImportIdentity(mnemonic, seedPassword, s.identityManager, func() error {
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

func (s *Service) purgeExpiredAttachmentUploads(now time.Time) {
	const ttl = 15 * time.Minute
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	for uploadID, session := range s.uploads {
		if now.Sub(session.UpdatedAt) > ttl {
			delete(s.uploads, uploadID)
		}
	}
}
