package daemonservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	daemoncomposition "aim-chat/go-backend/internal/composition/daemon"
	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/domains/contracts"
	identityapp "aim-chat/go-backend/internal/domains/identity"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/pkg/models"
)

const (
	legacyAccountID      = "legacy"
	accountRegistryFile  = "accounts.json"
	accountRegistryV1    = 1
	accountPrefix        = "acct"
	networkSwitchTimeout = 5 * time.Second
)

type persistedAccountRegistry struct {
	Version  int                    `json:"version"`
	ActiveID string                 `json:"active_id"`
	Accounts []persistedAccountMeta `json:"accounts"`
}

type persistedAccountMeta struct {
	ID        string    `json:"id"`
	RelPath   string    `json:"rel_path"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) accountRegistryPath() string {
	return filepath.Join(s.dataDir, accountRegistryFile)
}

func (s *Service) loadAccountRegistry() (persistedAccountRegistry, error) {
	path := s.accountRegistryPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return persistedAccountRegistry{
				Version:  accountRegistryV1,
				ActiveID: legacyAccountID,
				Accounts: []persistedAccountMeta{
					{ID: legacyAccountID, RelPath: ".", CreatedAt: time.Now().UTC()},
				},
			}, nil
		}
		return persistedAccountRegistry{}, err
	}
	var reg persistedAccountRegistry
	if err := json.Unmarshal(raw, &reg); err != nil {
		return persistedAccountRegistry{}, err
	}
	if reg.Version != accountRegistryV1 {
		return persistedAccountRegistry{}, fmt.Errorf("unsupported account registry version: %d", reg.Version)
	}
	if !slices.ContainsFunc(reg.Accounts, func(entry persistedAccountMeta) bool { return strings.TrimSpace(entry.ID) == legacyAccountID }) {
		reg.Accounts = append(reg.Accounts, persistedAccountMeta{
			ID:        legacyAccountID,
			RelPath:   ".",
			CreatedAt: time.Now().UTC(),
		})
	}
	if strings.TrimSpace(reg.ActiveID) == "" {
		reg.ActiveID = legacyAccountID
	}
	return reg, nil
}

func (s *Service) saveAccountRegistry(reg persistedAccountRegistry) error {
	reg.Version = accountRegistryV1
	path := s.accountRegistryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func (s *Service) findAccountMeta(reg persistedAccountRegistry, accountID string) (persistedAccountMeta, bool) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return persistedAccountMeta{}, false
	}
	for _, entry := range reg.Accounts {
		if strings.TrimSpace(entry.ID) == accountID {
			return entry, true
		}
	}
	return persistedAccountMeta{}, false
}

func (s *Service) resolveAccountDataDir(entry persistedAccountMeta) string {
	rel := filepath.Clean(strings.TrimSpace(entry.RelPath))
	if rel == "" || rel == "." {
		return s.dataDir
	}
	return filepath.Join(s.dataDir, rel)
}

func (s *Service) initializeAccountRegistry(secret string) error {
	reg, err := s.loadAccountRegistry()
	if err != nil {
		return err
	}
	if err := s.saveAccountRegistry(reg); err != nil {
		return err
	}
	s.storageSecret = secret
	s.currentProfileID = reg.ActiveID
	if reg.ActiveID == legacyAccountID {
		return nil
	}
	return s.activateAccountLocked(reg.ActiveID, false)
}

func (s *Service) reloadFromBundle(bundle daemoncomposition.StorageBundle, secret string) error {
	manager, err := identityapp.NewManager()
	if err != nil {
		return err
	}
	s.identityManager = manager
	s.messageStore = bundle.MessageStore
	s.sessionManager = crypto.NewSessionManager(bundle.SessionStore)
	s.attachmentStore = bundle.AttachmentStore
	s.identityState = identityapp.NewStateStore()
	s.identityState.Configure(bundle.IdentityPath, secret)
	if err := s.identityState.Bootstrap(s.identityManager); err != nil {
		return err
	}

	s.privacyCore.Configure(bundle.PrivacyPath, bundle.BlocklistPath, secret)
	settings, _, settingsErr, blocklistErr := s.privacyCore.BootstrapPartial()
	if settingsErr != nil {
		s.logger.Warn("privacy settings bootstrap failed, using defaults", "error", settingsErr.Error())
	}
	if blocklistErr != nil {
		s.logger.Warn("blocklist bootstrap failed, using empty list", "error", blocklistErr.Error())
	}
	if err := s.applyStoragePolicyFromSettings(settings); err != nil {
		return err
	}

	s.bootstrapStateStores(bundle, secret)

	s.identityCore = identityapp.NewService(
		s.identityManager,
		s.identityState,
		s.messageStore,
		s.sessionManager,
		s.attachmentStore,
		s.logger,
	)
	s.messagingCore = messagingapp.NewService(buildMessagingDeps(s))
	s.inboundMessagingCore = messagingapp.NewInboundService(buildInboundMessagingDeps(s))
	s.groupCore = s.groupUseCases()
	s.inboxCore = s.inboxUseCases()
	s.notifier.Reset()
	s.bindingLinkMu.Lock()
	s.bindingLinks = map[string]pendingNodeBindingLink{}
	s.bindingLinkMu.Unlock()
	return nil
}

func (s *Service) activateAccountLocked(accountID string, persistActive bool) error {
	reg, err := s.loadAccountRegistry()
	if err != nil {
		return err
	}
	entry, ok := s.findAccountMeta(reg, accountID)
	if !ok {
		return errors.New("account profile is not found")
	}
	profileDataDir := s.resolveAccountDataDir(entry)
	bundle, err := daemoncomposition.BuildStorageBundle(profileDataDir, s.storageSecret)
	if err != nil {
		return err
	}
	if err := s.reloadFromBundle(bundle, s.storageSecret); err != nil {
		return err
	}
	s.currentProfileID = entry.ID
	if persistActive {
		reg.ActiveID = entry.ID
		if err := s.saveAccountRegistry(reg); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) createAccountProfileLocked() (persistedAccountMeta, persistedAccountRegistry, error) {
	reg, err := s.loadAccountRegistry()
	if err != nil {
		return persistedAccountMeta{}, persistedAccountRegistry{}, err
	}
	profileID, err := runtimeapp.GeneratePrefixedID(accountPrefix)
	if err != nil {
		return persistedAccountMeta{}, persistedAccountRegistry{}, err
	}
	profileID = strings.ReplaceAll(strings.TrimSpace(profileID), "-", "_")
	relPath := filepath.Join("profiles", profileID)
	entry := persistedAccountMeta{
		ID:        profileID,
		RelPath:   relPath,
		CreatedAt: time.Now().UTC(),
	}
	reg.Accounts = append(reg.Accounts, entry)
	if err := s.saveAccountRegistry(reg); err != nil {
		return persistedAccountMeta{}, persistedAccountRegistry{}, err
	}
	return entry, reg, nil
}

func (s *Service) removeAccountProfileLocked(accountID string) error {
	reg, err := s.loadAccountRegistry()
	if err != nil {
		return err
	}
	next := reg.Accounts[:0]
	for _, entry := range reg.Accounts {
		if strings.TrimSpace(entry.ID) == accountID {
			continue
		}
		next = append(next, entry)
	}
	reg.Accounts = next
	if reg.ActiveID == accountID {
		reg.ActiveID = legacyAccountID
	}
	if err := s.saveAccountRegistry(reg); err != nil {
		return err
	}
	return nil
}

func (s *Service) ListAccounts() ([]contracts.AccountProfile, error) {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	reg, err := s.loadAccountRegistry()
	if err != nil {
		return nil, err
	}
	out := make([]contracts.AccountProfile, 0, len(reg.Accounts))
	for _, entry := range reg.Accounts {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		out = append(out, contracts.AccountProfile{
			ID:     id,
			Active: id == strings.TrimSpace(reg.ActiveID),
		})
	}
	return out, nil
}

func (s *Service) GetCurrentAccount() (contracts.AccountProfile, error) {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	reg, err := s.loadAccountRegistry()
	if err != nil {
		return contracts.AccountProfile{}, err
	}
	return contracts.AccountProfile{
		ID:     strings.TrimSpace(reg.ActiveID),
		Active: true,
	}, nil
}

func (s *Service) SwitchAccount(accountID string) (models.Identity, error) {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return models.Identity{}, errors.New("account id is required")
	}
	if accountID == s.currentProfileID {
		return s.identityManager.GetIdentity(), nil
	}

	wasRunning := s.runtime.IsNetworking()
	if wasRunning {
		stopCtx, cancel := context.WithTimeout(context.Background(), networkSwitchTimeout)
		_ = s.StopNetworking(stopCtx)
		cancel()
	}

	previousAccountID := s.currentProfileID
	if err := s.activateAccountLocked(accountID, true); err != nil {
		if previousAccountID != "" {
			_ = s.activateAccountLocked(previousAccountID, false)
		}
		if wasRunning {
			_ = s.StartNetworking(context.Background())
		}
		return models.Identity{}, err
	}

	if wasRunning {
		if err := s.StartNetworking(context.Background()); err != nil {
			return models.Identity{}, err
		}
	}
	return s.identityManager.GetIdentity(), nil
}

func (s *Service) CreateIdentity(password string) (models.Identity, string, error) {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	return s.runIdentityOpOnNewAccountLocked(func() (models.Identity, string, error) {
		return identityapp.CreateIdentity(password, s.identityManager, func() error {
			return s.identityState.Persist(s.identityManager)
		})
	})
}

func (s *Service) ImportIdentity(mnemonic, password string) (models.Identity, error) {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	created, _, err := s.runIdentityOpOnNewAccountLocked(func() (models.Identity, string, error) {
		identity, importErr := identityapp.ImportIdentity(mnemonic, password, s.identityManager, func() error {
			return s.identityState.Persist(s.identityManager)
		})
		return identity, "", importErr
	})
	if err != nil {
		return models.Identity{}, err
	}
	return created, nil
}

func (s *Service) runIdentityOpOnNewAccountLocked(op func() (models.Identity, string, error)) (models.Identity, string, error) {
	entry, _, err := s.createAccountProfileLocked()
	if err != nil {
		return models.Identity{}, "", err
	}
	previousAccountID := s.currentProfileID
	wasRunning := s.runtime.IsNetworking()
	if wasRunning {
		stopCtx, cancel := context.WithTimeout(context.Background(), networkSwitchTimeout)
		_ = s.StopNetworking(stopCtx)
		cancel()
	}

	restoreAndRestart := func() {
		if previousAccountID != "" {
			_ = s.activateAccountLocked(previousAccountID, true)
		}
		if wasRunning {
			_ = s.StartNetworking(context.Background())
		}
	}

	if err := s.activateAccountLocked(entry.ID, true); err != nil {
		_ = s.removeAccountProfileLocked(entry.ID)
		restoreAndRestart()
		return models.Identity{}, "", err
	}

	created, mnemonic, err := op()
	if err != nil {
		_ = s.removeAccountProfileLocked(entry.ID)
		restoreAndRestart()
		return models.Identity{}, "", err
	}

	if wasRunning {
		if err := s.StartNetworking(context.Background()); err != nil {
			return models.Identity{}, "", err
		}
	}
	return created, mnemonic, nil
}
