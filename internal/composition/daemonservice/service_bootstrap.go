package daemonservice

import (
	daemoncomposition "aim-chat/go-backend/internal/composition/daemon"
	"aim-chat/go-backend/internal/domains/contracts"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/internal/waku"
)

// noinspection GoUnusedExportedFunction
func NewServiceForDaemon(wakuCfg waku.Config) (*Service, error) {
	return NewServiceForDaemonWithDataDir(wakuCfg, "")
}

func NewServiceForDaemonWithDataDir(wakuCfg waku.Config, dataDir string) (*Service, error) {
	resolvedDir, secret, bundle, err := daemoncomposition.ResolveStorage(dataDir)
	if err != nil {
		return nil, err
	}
	return newServiceForDaemonWithBundle(wakuCfg, bundle, secret, resolvedDir)
}

func newServiceForDaemonWithBundle(wakuCfg waku.Config, bundle daemoncomposition.StorageBundle, secret, dataDir string) (*Service, error) {
	svc, err := newServiceWithOptions(wakuCfg, contracts.ServiceOptions{
		SessionStore:    bundle.SessionStore,
		MessageStore:    bundle.MessageStore,
		AttachmentStore: bundle.AttachmentStore,
		Logger:          runtimeapp.DefaultLogger(),
	})
	if err != nil {
		return nil, err
	}
	svc.identityState.Configure(bundle.IdentityPath, secret)
	if err := svc.identityState.Bootstrap(svc.identityManager); err != nil {
		return nil, err
	}
	svc.privacyCore.Configure(bundle.PrivacyPath, bundle.BlocklistPath, secret)
	settings, _, settingsErr, blocklistErr := svc.privacyCore.BootstrapPartial()
	if settingsErr != nil {
		svc.logger.Warn("privacy settings bootstrap failed, using defaults", "error", settingsErr.Error())
	}
	if blocklistErr != nil {
		svc.logger.Warn("blocklist bootstrap failed, using empty list", "error", blocklistErr.Error())
	}
	if err := svc.applyStoragePolicyFromSettings(settings); err != nil {
		return nil, err
	}
	svc.applyNodePoliciesFromSettings(settings)
	svc.bootstrapStateStores(bundle, secret)
	svc.storageSecret = secret
	svc.dataDir = dataDir
	svc.currentProfileID = legacyAccountID
	if err := svc.initializeAccountRegistry(secret); err != nil {
		return nil, err
	}
	if err := svc.configureEnrollmentTokenFlow(); err != nil {
		return nil, err
	}
	return svc, nil
}
