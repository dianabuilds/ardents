package daemonservice

import (
	"errors"
	"time"

	groupdomain "aim-chat/go-backend/internal/domains/group"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

var ErrBackupDisabledByRetentionPolicy = errors.New("backup export is disabled in zero-retention mode")

func (s *Service) GetStoragePolicy() (privacydomain.StoragePolicy, error) {
	return s.privacyCore.GetStoragePolicy()
}

func (s *Service) SetStorageScopeOverride(
	scope string,
	scopeID string,
	storageProtection string,
	retention string,
	messageTTLSeconds int,
	imageTTLSeconds int,
	fileTTLSeconds int,
	imageQuotaMB int,
	fileQuotaMB int,
	imageMaxItemSizeMB int,
	fileMaxItemSizeMB int,
	infiniteTTL bool,
	pinRequiredForInfinite bool,
) (privacydomain.StoragePolicyOverride, error) {
	policy, err := privacydomain.ParseStoragePolicy(
		storageProtection,
		retention,
		messageTTLSeconds,
		imageTTLSeconds,
		fileTTLSeconds,
		imageQuotaMB,
		fileQuotaMB,
		imageMaxItemSizeMB,
		fileMaxItemSizeMB,
	)
	if err != nil {
		return privacydomain.StoragePolicyOverride{}, err
	}
	return s.privacyCore.SetStorageScopeOverride(scope, scopeID, privacydomain.StoragePolicyOverride{
		StorageProtection:      policy.StorageProtection,
		ContentRetentionMode:   policy.ContentRetentionMode,
		MessageTTLSeconds:      policy.MessageTTLSeconds,
		ImageTTLSeconds:        policy.ImageTTLSeconds,
		FileTTLSeconds:         policy.FileTTLSeconds,
		ImageQuotaMB:           policy.ImageQuotaMB,
		FileQuotaMB:            policy.FileQuotaMB,
		ImageMaxItemSizeMB:     policy.ImageMaxItemSizeMB,
		FileMaxItemSizeMB:      policy.FileMaxItemSizeMB,
		InfiniteTTL:            infiniteTTL,
		PinRequiredForInfinite: pinRequiredForInfinite,
	})
}

func (s *Service) GetStorageScopeOverride(scope string, scopeID string) (privacydomain.StoragePolicyOverride, bool, error) {
	return s.privacyCore.GetStorageScopeOverride(scope, scopeID)
}

func (s *Service) RemoveStorageScopeOverride(scope string, scopeID string) (bool, error) {
	return s.privacyCore.RemoveStorageScopeOverride(scope, scopeID)
}

func (s *Service) ResolveStoragePolicy(scope string, scopeID string, isPinned bool) (privacydomain.StoragePolicy, error) {
	return s.privacyCore.ResolveStoragePolicy(scope, scopeID, isPinned)
}

func (s *Service) UpdateStoragePolicy(
	storageProtection string,
	retention string,
	messageTTLSeconds int,
	imageTTLSeconds int,
	fileTTLSeconds int,
	imageQuotaMB int,
	fileQuotaMB int,
	imageMaxItemSizeMB int,
	fileMaxItemSizeMB int,
) (privacydomain.StoragePolicy, error) {
	currentSettings, err := s.privacyCore.GetPrivacySettings()
	if err != nil {
		return privacydomain.StoragePolicy{}, err
	}
	previousPolicy := privacydomain.StoragePolicyFromSettings(currentSettings)

	policy, err := privacydomain.ParseStoragePolicy(
		storageProtection,
		retention,
		messageTTLSeconds,
		imageTTLSeconds,
		fileTTLSeconds,
		imageQuotaMB,
		fileQuotaMB,
		imageMaxItemSizeMB,
		fileMaxItemSizeMB,
	)
	if err != nil {
		return privacydomain.StoragePolicy{}, err
	}
	if applyErr := s.applyStoragePolicy(policy); applyErr != nil {
		return privacydomain.StoragePolicy{}, applyErr
	}
	updatedPolicy, persistErr := s.privacyCore.UpdateStoragePolicy(
		storageProtection,
		retention,
		messageTTLSeconds,
		imageTTLSeconds,
		fileTTLSeconds,
		imageQuotaMB,
		fileQuotaMB,
		imageMaxItemSizeMB,
		fileMaxItemSizeMB,
	)
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
	if setter, ok := s.attachmentStore.(interface {
		SetClassPolicies(imageQuotaMB, imageMaxItemSizeMB, fileQuotaMB, fileMaxItemSizeMB int)
	}); ok {
		setter.SetClassPolicies(
			policy.ImageQuotaMB,
			policy.ImageMaxItemSizeMB,
			policy.FileQuotaMB,
			policy.FileMaxItemSizeMB,
		)
	}
	if setter, ok := s.attachmentStore.(interface {
		SetHardCapPolicy(highWatermarkPercent, fullCapPercent, aggressiveTargetPercent int)
	}); ok {
		s.presetMu.RLock()
		cfg := s.nodePreset
		s.presetMu.RUnlock()
		setter.SetHardCapPolicy(cfg.HighWatermarkPercent, cfg.FullCapPercent, cfg.AggressiveTargetPercent)
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

func (s *Service) RunAttachmentGCDryRun(now time.Time) (storage.AttachmentGCReport, error) {
	policy, err := s.GetStoragePolicy()
	if err != nil {
		return storage.AttachmentGCReport{}, err
	}
	imageTTL := 0
	fileTTL := 0
	if policy.ContentRetentionMode == privacydomain.RetentionEphemeral {
		imageTTL = policy.ImageTTLSeconds
		fileTTL = policy.FileTTLSeconds
	}
	gcRunner, ok := s.attachmentStore.(interface {
		RunGC(now time.Time, imageTTLSeconds, fileTTLSeconds int, dryRun bool) (storage.AttachmentGCReport, error)
	})
	if !ok {
		return storage.AttachmentGCReport{}, errors.New("attachment gc is not supported")
	}
	return gcRunner.RunGC(now, imageTTL, fileTTL, true)
}

func (s *Service) enforceRetentionPolicies(now time.Time) {
	policy, err := s.GetStoragePolicy()
	if err != nil {
		s.recordError("storage_policy", err)
		return
	}
	if policy.ContentRetentionMode == privacydomain.RetentionEphemeral && policy.MessageTTLSeconds > 0 {
		cutoff := now.Add(-time.Duration(policy.MessageTTLSeconds) * time.Second)
		if purger, ok := s.messageStore.(interface {
			PurgeOlderThan(time.Time) (int, error)
		}); ok {
			if _, err := purger.PurgeOlderThan(cutoff); err != nil {
				s.recordError("storage", err)
			}
		}
	}
	imageTTL := 0
	fileTTL := 0
	if policy.ContentRetentionMode == privacydomain.RetentionEphemeral {
		imageTTL = policy.ImageTTLSeconds
		fileTTL = policy.FileTTLSeconds
	}
	if gcRunner, ok := s.attachmentStore.(interface {
		RunGC(now time.Time, imageTTLSeconds, fileTTLSeconds int, dryRun bool) (storage.AttachmentGCReport, error)
	}); ok {
		report, err := gcRunner.RunGC(now, imageTTL, fileTTL, false)
		if err != nil {
			s.recordError("storage", err)
			return
		}
		if report.DeletedCount > 0 {
			s.recordGCEvictions(report.DeletedByClass)
		}
		return
	}
	// Legacy fallback.
	if policy.ContentRetentionMode == privacydomain.RetentionEphemeral && (imageTTL > 0 || fileTTL > 0) {
		if purger, ok := s.attachmentStore.(interface {
			PurgeOlderThanByClass(class string, cutoff time.Time) (int, error)
		}); ok {
			if imageTTL > 0 {
				cutoff := now.Add(-time.Duration(imageTTL) * time.Second)
				if _, err := purger.PurgeOlderThanByClass("image", cutoff); err != nil {
					s.recordError("storage", err)
				}
			}
			if fileTTL > 0 {
				cutoff := now.Add(-time.Duration(fileTTL) * time.Second)
				if _, err := purger.PurgeOlderThanByClass("file", cutoff); err != nil {
					s.recordError("storage", err)
				}
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
	wipeErr = errors.Join(wipeErr, wipeIfSupported(s.bindingStore))
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
	if s.bindingLinkMu != nil {
		s.bindingLinkMu.Lock()
		s.bindingLinks = map[string]pendingNodeBindingLink{}
		s.bindingLinkMu.Unlock()
	}
}
