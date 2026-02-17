package usecase

import (
	"sync"

	privacymodel "aim-chat/go-backend/internal/domains/privacy/model"
)

type PrivacySettingsStateStore interface {
	Configure(path, secret string)
	Bootstrap() (privacymodel.PrivacySettings, error)
	Persist(settings privacymodel.PrivacySettings) error
}

type BlocklistStateStore interface {
	Configure(path, secret string)
	Bootstrap() (privacymodel.Blocklist, error)
	Persist(list privacymodel.Blocklist) error
}

type Service struct {
	privacyState   PrivacySettingsStateStore
	blocklistState BlocklistStateStore
	recordError    func(string, error)

	mu        sync.RWMutex
	privacy   privacymodel.PrivacySettings
	blocklist privacymodel.Blocklist
}

func NewService(
	privacyState PrivacySettingsStateStore,
	blocklistState BlocklistStateStore,
	recordError func(string, error),
) *Service {
	return &Service{
		privacyState:   privacyState,
		blocklistState: blocklistState,
		recordError:    recordError,
		privacy:        privacymodel.DefaultPrivacySettings(),
		blocklist:      privacymodel.Blocklist{},
	}
}

func (s *Service) Configure(privacyPath, blocklistPath, secret string) {
	s.privacyState.Configure(privacyPath, secret)
	s.blocklistState.Configure(blocklistPath, secret)
}

func (s *Service) SetState(settings privacymodel.PrivacySettings, blocklist privacymodel.Blocklist) {
	s.mu.Lock()
	s.privacy = privacymodel.NormalizePrivacySettings(settings)
	s.blocklist = blocklist
	s.mu.Unlock()
}

func (s *Service) Bootstrap() (privacymodel.PrivacySettings, privacymodel.Blocklist, error) {
	settings, err := s.privacyState.Bootstrap()
	if err != nil {
		return privacymodel.PrivacySettings{}, privacymodel.Blocklist{}, err
	}
	list, err := s.blocklistState.Bootstrap()
	if err != nil {
		return privacymodel.PrivacySettings{}, privacymodel.Blocklist{}, err
	}
	settings = privacymodel.NormalizePrivacySettings(settings)
	s.SetState(settings, list)
	return settings, list, nil
}

func (s *Service) CurrentMode() privacymodel.MessagePrivacyMode {
	s.mu.RLock()
	mode := s.privacy.MessagePrivacyMode
	s.mu.RUnlock()
	return mode
}

func (s *Service) IsBlockedSender(senderID string) bool {
	s.mu.RLock()
	blocked := s.blocklist.Contains(senderID)
	s.mu.RUnlock()
	return blocked
}

func (s *Service) GetPrivacySettings() (privacymodel.PrivacySettings, error) {
	s.mu.RLock()
	settings := s.privacy
	s.mu.RUnlock()
	return privacymodel.NormalizePrivacySettings(settings), nil
}

func (s *Service) UpdatePrivacySettings(mode string) (privacymodel.PrivacySettings, error) {
	parsedMode, err := privacymodel.ParseMessagePrivacyMode(mode)
	if err != nil {
		return privacymodel.PrivacySettings{}, err
	}
	updated := privacymodel.PrivacySettings{MessagePrivacyMode: parsedMode}
	updated = privacymodel.NormalizePrivacySettings(updated)
	if err := s.privacyState.Persist(updated); err != nil {
		if s.recordError != nil {
			s.recordError("storage", err)
		}
		return privacymodel.PrivacySettings{}, err
	}

	s.mu.Lock()
	s.privacy = updated
	s.mu.Unlock()
	return updated, nil
}

func (s *Service) GetBlocklist() ([]string, error) {
	s.mu.RLock()
	out := s.blocklist.List()
	s.mu.RUnlock()
	return out, nil
}

func (s *Service) AddToBlocklist(identityID string) ([]string, error) {
	return s.updateBlocklist(func(next privacymodel.Blocklist) error {
		return next.Add(identityID)
	})
}

func (s *Service) RemoveFromBlocklist(identityID string) ([]string, error) {
	return s.updateBlocklist(func(next privacymodel.Blocklist) error {
		return next.Remove(identityID)
	})
}

func (s *Service) updateBlocklist(mutate func(privacymodel.Blocklist) error) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next, err := privacymodel.NewBlocklist(s.blocklist.List())
	if err != nil {
		return nil, err
	}
	if err := mutate(next); err != nil {
		return nil, err
	}
	if err := s.blocklistState.Persist(next); err != nil {
		if s.recordError != nil {
			s.recordError("storage", err)
		}
		return nil, err
	}
	s.blocklist = next
	return next.List(), nil
}
