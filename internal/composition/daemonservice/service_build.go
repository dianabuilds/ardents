package daemonservice

import (
	"log/slog"
	"sync"

	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	identityapp "aim-chat/go-backend/internal/domains/identity"
	inboxapp "aim-chat/go-backend/internal/domains/inbox"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	privacyapp "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/internal/platform/privacylog"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
)

func newServiceWithOptions(wakuCfg waku.Config, opts contracts.ServiceOptions) (*Service, error) {
	opts, err := ensureServiceOptions(opts)
	if err != nil {
		return nil, err
	}

	manager, err := identityapp.NewManager()
	if err != nil {
		return nil, err
	}

	privacyStore := privacyapp.NewSettingsStore()
	blocklistStore := privacyapp.NewBlocklistStore()
	requestRuntime := inboxapp.NewRuntimeState()
	groupRuntime := groupdomain.NewRuntimeState()
	defaultPreset := defaultBlobNodePresetConfig()
	svc := &Service{
		identityManager:   manager,
		wakuNode:          waku.NewNode(wakuCfg),
		sessionManager:    crypto.NewSessionManager(opts.SessionStore),
		messageStore:      opts.MessageStore,
		attachmentStore:   opts.AttachmentStore,
		notifier:          runtimeapp.NewNotificationHub(2048),
		logger:            opts.Logger,
		metrics:           runtimeapp.NewServiceMetricsState(),
		runtime:           runtimeapp.NewServiceRuntime(),
		requestRuntime:    requestRuntime,
		groupRuntime:      groupRuntime,
		identityState:     identityapp.NewStateStore(),
		privacyState:      privacyStore,
		requestInboxState: inboxapp.NewRequestStore(),
		groupStateStore:   groupdomain.NewSnapshotStore(),
		groupAbuse:        groupdomain.NewAbuseProtectionFromEnv(),
		startStopMu:       &sync.Mutex{},
		metaHardening:     newOutboundMetadataHardeningFromEnv(),
		replicationMu:     &sync.RWMutex{},
		replicationMode:   resolveBlobReplicationModeFromEnv(),
		blobFlags:         resolveBlobFeatureFlagsFromEnv(),
		presetMu:          &sync.RWMutex{},
		nodePreset:        defaultPreset,
		serveLimiter:      newBandwidthLimiter(defaultPreset.ServeBandwidthKBps),
		fetchLimiter:      newBandwidthLimiter(defaultPreset.FetchBandwidthKBps),
		blobACLMu:         &sync.RWMutex{},
		blobACL:           resolveBlobACLPolicyFromEnv(),
		bindingStore:      newNodeBindingStore(),
		bindingLinkMu:     &sync.Mutex{},
		bindingLinks:      map[string]pendingNodeBindingLink{},
		blobProviders:     newBlobProviderRegistry(),
	}

	svc.identityCore = identityapp.NewService(
		svc.identityManager,
		svc.identityState,
		svc.messageStore,
		svc.sessionManager,
		svc.attachmentStore,
		svc.logger,
	)
	svc.privacyCore = privacyapp.NewService(privacyStore, blocklistStore, svc.recordError)
	svc.messagingCore = messagingapp.NewService(buildMessagingDeps(svc))
	svc.inboundMessagingCore = messagingapp.NewInboundService(buildInboundMessagingDeps(svc))
	svc.groupCore = svc.groupUseCases()
	svc.inboxCore = svc.inboxUseCases()
	return svc, nil
}

func ensureServiceOptions(opts contracts.ServiceOptions) (contracts.ServiceOptions, error) {
	var err error
	if opts.SessionStore == nil {
		opts.SessionStore = crypto.NewInMemorySessionStore()
	}
	if opts.MessageStore == nil {
		opts.MessageStore = storage.NewMessageStore()
	}
	if opts.Logger == nil {
		opts.Logger = runtimeapp.DefaultLogger()
	}
	opts.Logger = slog.New(privacylog.WrapHandler(opts.Logger.Handler()))
	if opts.AttachmentStore == nil {
		opts.AttachmentStore, err = storage.NewAttachmentStore("")
		if err != nil {
			return contracts.ServiceOptions{}, err
		}
	}
	return opts, nil
}
