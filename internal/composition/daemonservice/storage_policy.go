package daemonservice

import (
	"errors"
	"time"

	groupdomain "aim-chat/go-backend/internal/domains/group"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/pkg/models"
)

var ErrBackupDisabledByRetentionPolicy = errors.New("backup export is disabled in zero-retention mode")

func (s *Service) GetStoragePolicy() (privacydomain.StoragePolicy, error) {
	return s.privacyCore.GetStoragePolicy()
}

func (s *Service) UpdateStoragePolicy(
	storageProtection string,
	retention string,
	messageTTLSeconds int,
	fileTTLSeconds int,
) (privacydomain.StoragePolicy, error) {
	currentSettings, err := s.privacyCore.GetPrivacySettings()
	if err != nil {
		return privacydomain.StoragePolicy{}, err
	}
	previousPolicy := privacydomain.StoragePolicyFromSettings(currentSettings)

	policy, err := privacydomain.ParseStoragePolicy(storageProtection, retention, messageTTLSeconds, fileTTLSeconds)
	if err != nil {
		return privacydomain.StoragePolicy{}, err
	}
	if applyErr := s.applyStoragePolicy(policy); applyErr != nil {
		return privacydomain.StoragePolicy{}, applyErr
	}
	updatedPolicy, persistErr := s.privacyCore.UpdateStoragePolicy(storageProtection, retention, messageTTLSeconds, fileTTLSeconds)
	if persistErr != nil {
		// Best-effort rollback of in-memory runtime toggles when persistence fails.
		_ = s.applyStoragePolicy(previousPolicy)
		return privacydomain.StoragePolicy{}, persistErr
	}
	return updatedPolicy, nil
}

func (s *Service) ExportBackup(consentToken, passphrase string) (string, error) {
	settings, err := s.privacyCore.GetPrivacySettings()
	if err != nil {
		return "", err
	}
	if settings.ContentRetentionMode == privacydomain.RetentionZeroRetention {
		return "", ErrBackupDisabledByRetentionPolicy
	}
	return s.identityCore.ExportBackup(consentToken, passphrase)
}

func (s *Service) applyStoragePolicy(policy privacydomain.StoragePolicy) error {
	policy = privacydomain.NormalizeStoragePolicy(policy)
	persistentContentAllowed := policy.ContentRetentionMode != privacydomain.RetentionZeroRetention

	if setter, ok := s.messageStore.(interface{ SetPersistenceEnabled(bool) }); ok {
		setter.SetPersistenceEnabled(persistentContentAllowed)
	}
	if setter, ok := s.attachmentStore.(interface{ SetPersistenceEnabled(bool) }); ok {
		setter.SetPersistenceEnabled(persistentContentAllowed)
	}
	if setter, ok := s.sessionManager.(interface{ SetPersistenceEnabled(bool) }); ok {
		setter.SetPersistenceEnabled(persistentContentAllowed)
	}

	if !persistentContentAllowed {
		return s.wipeContentState()
	}
	return nil
}

func (s *Service) applyStoragePolicyFromSettings(settings privacydomain.PrivacySettings) error {
	return s.applyStoragePolicy(privacydomain.StoragePolicyFromSettings(settings))
}

func (s *Service) enforceRetentionPolicies(now time.Time) {
	policy, err := s.GetStoragePolicy()
	if err != nil {
		s.recordError("storage_policy", err)
		return
	}
	if policy.ContentRetentionMode != privacydomain.RetentionEphemeral {
		return
	}
	if policy.MessageTTLSeconds > 0 {
		cutoff := now.Add(-time.Duration(policy.MessageTTLSeconds) * time.Second)
		if purger, ok := s.messageStore.(interface {
			PurgeOlderThan(time.Time) (int, error)
		}); ok {
			if _, err := purger.PurgeOlderThan(cutoff); err != nil {
				s.recordError("storage", err)
			}
		}
	}
	if policy.FileTTLSeconds > 0 {
		cutoff := now.Add(-time.Duration(policy.FileTTLSeconds) * time.Second)
		if purger, ok := s.attachmentStore.(interface {
			PurgeOlderThan(time.Time) (int, error)
		}); ok {
			if _, err := purger.PurgeOlderThan(cutoff); err != nil {
				s.recordError("storage", err)
			}
		}
	}
}

func (s *Service) cleanupOnStopIfZeroRetention() error {
	policy, err := s.GetStoragePolicy()
	if err != nil {
		return err
	}
	if policy.ContentRetentionMode != privacydomain.RetentionZeroRetention {
		return nil
	}
	return s.wipeContentState()
}

func (s *Service) wipeContentState() error {
	var wipeErr error
	wipeErr = errors.Join(wipeErr, wipeIfSupported(s.messageStore))
	wipeErr = errors.Join(wipeErr, wipeIfSupported(s.sessionManager))
	wipeErr = errors.Join(wipeErr, wipeIfSupported(s.attachmentStore))
	if s.requestInboxState != nil {
		wipeErr = errors.Join(wipeErr, s.requestInboxState.Wipe())
	}
	if s.groupStateStore != nil {
		wipeErr = errors.Join(wipeErr, s.groupStateStore.Wipe())
	}
	s.resetVolatileRuntimeState()
	return wipeErr
}

func wipeIfSupported(target any) error {
	wiper, ok := target.(interface{ Wipe() error })
	if !ok {
		return nil
	}
	return wiper.Wipe()
}

func (s *Service) resetVolatileRuntimeState() {
	if s.requestRuntime != nil {
		s.requestRuntime.Mu.Lock()
		s.requestRuntime.SetInbox(map[string][]models.Message{})
		s.requestRuntime.Mu.Unlock()
	}
	if s.groupRuntime != nil {
		s.groupRuntime.StateMu.Lock()
		s.groupRuntime.SetSnapshot(map[string]groupdomain.GroupState{}, map[string][]groupdomain.GroupEvent{})
		s.groupRuntime.StateMu.Unlock()
		s.groupRuntime.ReplayMu.Lock()
		s.groupRuntime.ReplaySeen = make(map[string]time.Time)
		s.groupRuntime.ReplayMu.Unlock()
	}
	if s.notifier != nil {
		s.notifier.Reset()
	}
}
