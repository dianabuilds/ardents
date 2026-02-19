package daemonservice

import (
	"time"

	daemoncomposition "aim-chat/go-backend/internal/composition/daemon"
	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	inboxapp "aim-chat/go-backend/internal/domains/inbox"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

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

func newServiceForDaemonWithBundle(wakuCfg waku.Config, bundle daemoncomposition.StorageBundle, secret string) (*Service, error) {
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
	svc.requestInboxState.Configure(bundle.RequestInboxPath, secret)
	inbox, err := svc.requestInboxState.Bootstrap()
	if err != nil {
		svc.logger.Warn("message request inbox bootstrap failed, using empty list", "error", err.Error())
		inbox = map[string][]models.Message{}
	}
	svc.requestRuntime.SetInbox(inboxapp.CopyInboxState(inbox))
	svc.groupStateStore.Configure(bundle.GroupStatePath, secret)
	groupStates, groupEventLog, err := svc.groupStateStore.Bootstrap()
	if err != nil {
		svc.logger.Warn("group state bootstrap failed, using empty state", "error", err.Error())
		groupStates = map[string]groupdomain.GroupState{}
		groupEventLog = map[string][]groupdomain.GroupEvent{}
	}
	svc.groupRuntime.SetSnapshot(groupStates, groupEventLog)
	if svc.groupRuntime.ReplaySeen == nil {
		svc.groupRuntime.ReplaySeen = make(map[string]time.Time)
	}
	svc.bindingStore.Configure(bundle.NodeBindingPath, secret)
	if err := svc.bindingStore.Bootstrap(); err != nil {
		svc.logger.Warn("node binding bootstrap failed, using empty state", "error", err.Error())
	}
	return svc, nil
}
