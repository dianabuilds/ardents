package daemonservice

import (
	"crypto/ed25519"
	"log/slog"
	"sync"
	"time"

	"aim-chat/go-backend/internal/bootstrap/bootstrapmanager"
	"aim-chat/go-backend/internal/bootstrap/enrollmenttoken"
	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	identityapp "aim-chat/go-backend/internal/domains/identity"
	inboxapp "aim-chat/go-backend/internal/domains/inbox"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	privacyapp "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/internal/platform/ratelimiter"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

type identityCore = identityapp.Service
type privacyCore = privacyapp.Service
type messagingCore = messagingapp.Service
type inboundMessagingCore = messagingapp.InboundService
type groupCore interface {
	CreateGroup(title string) (groupdomain.Group, error)
	GetGroup(groupID string) (groupdomain.Group, error)
	ListGroups() ([]groupdomain.Group, error)
	UpdateGroupTitle(groupID, title string) (groupdomain.Group, error)
	UpdateGroupProfile(groupID, title, description, avatar string) (groupdomain.Group, error)
	DeleteGroup(groupID string) (bool, error)
	ListGroupMembers(groupID string) ([]groupdomain.GroupMember, error)
	LeaveGroup(groupID string) (bool, error)
	InviteToGroup(groupID, memberID string) (groupdomain.GroupMember, error)
	AcceptGroupInvite(groupID string) (bool, error)
	DeclineGroupInvite(groupID string) (bool, error)
	RemoveGroupMember(groupID, memberID string) (bool, error)
	PromoteGroupMember(groupID, memberID string) (groupdomain.GroupMember, error)
	DemoteGroupMember(groupID, memberID string) (groupdomain.GroupMember, error)
	SendGroupMessage(groupID, content string) (groupdomain.GroupMessageFanoutResult, error)
	SendGroupMessageInThread(groupID, content, threadID string) (groupdomain.GroupMessageFanoutResult, error)
	ListGroupMessages(groupID string, limit, offset int) ([]models.Message, error)
	ListGroupMessagesByThread(groupID, threadID string, limit, offset int) ([]models.Message, error)
	GetGroupMessageStatus(groupID, messageID string) (models.MessageStatus, error)
	DeleteGroupMessage(groupID, messageID string) error
}

type inboxCore interface {
	ListMessageRequests() ([]models.MessageRequest, error)
	GetMessageRequest(senderID string) (models.MessageRequestThread, error)
	AcceptMessageRequest(senderID string) (bool, error)
	DeclineMessageRequest(senderID string) (bool, error)
	BlockSender(senderID string) (models.BlockSenderResult, error)
}

type Service struct {
	identityManager contracts.IdentityDomain
	wakuNode        contracts.TransportNode
	sessionManager  contracts.SessionDomain
	messageStore    contracts.MessageRepository
	attachmentStore contracts.AttachmentRepository
	notifier        *runtimeapp.NotificationHub
	logger          *slog.Logger
	*identityCore
	*privacyCore
	*messagingCore
	*inboundMessagingCore
	groupCore
	inboxCore
	metrics            *runtimeapp.ServiceMetricsState
	runtime            *runtimeapp.ServiceRuntime
	requestRuntime     *inboxapp.RuntimeState
	groupRuntime       *groupdomain.RuntimeState
	identityState      *identityapp.StateStore
	privacyState       *privacyapp.SettingsStore
	requestInboxState  *inboxapp.RequestStore
	groupStateStore    *groupdomain.SnapshotStore
	groupAbuse         *groupdomain.AbuseProtection
	startStopMu        *sync.Mutex
	metaHardening      *outboundMetadataHardening
	replicationMu      *sync.RWMutex
	replicationMode    blobReplicationMode
	blobFlags          blobFeatureFlags
	presetMu           *sync.RWMutex
	nodePreset         blobNodePresetConfig
	serveSoftLimiter   *bandwidthLimiter
	serveLimiter       *bandwidthLimiter
	fetchLimiter       *bandwidthLimiter
	serveGuardMu       *sync.Mutex
	serveInFlight      int
	serveMaxConcurrent int
	servePerPeerPerMin int
	servePeerLimiter   *ratelimiter.MapLimiter
	publicBlobCache    *publicEphemeralBlobCache
	degradeMu          *sync.Mutex
	degradeState       publicServingDegradeState
	degradeCfg         publicServingDegradeConfig
	diagEventsMu       *sync.Mutex
	diagEvents         []diagnosticEventEntry
	blobACLMu          *sync.RWMutex
	blobACL            blobACLPolicy
	bindingStore       *nodeBindingStore
	bindingLinkMu      *sync.Mutex
	bindingLinks       map[string]pendingNodeBindingLink
	blobProviders      *blobProviderRegistry
	wakuCfg            *waku.Config
	bootstrapManager   *bootstrapmanager.Manager
	bootstrapRefresher *bootstrapmanager.Refresher
	bootstrapCancel    func()
	bootstrapWG        sync.WaitGroup
	dataDir            string
	storageSecret      string
	currentProfileID   string
	profileMu          *sync.Mutex
	enrollmentStore    *enrollmenttoken.FileStore
	enrollmentKeys     map[string]ed25519.PublicKey
}

type publicServingDegradeConfig struct {
	Enabled                   bool
	OverloadWindow            time.Duration
	RecoveryWindow            time.Duration
	RetryLoopLagThreshold     time.Duration
	PendingQueueThreshold     int
	RAMAllocThresholdMB       int
	DegradedServeFactorPct    int
	DegradedConcurrentServes  int
	DegradedPerPeerRequestsPM int
}

type publicServingDegradeState struct {
	OverloadSince     time.Time
	StableSince       time.Time
	SoftCapExceeded   bool
	Degraded          bool
	LastReason        string
	BaseServeSoftKBps int
	BaseServeHardKBps int
	BaseConcurrent    int
	BasePerPeerPerMin int
	DegradedAppliedAt time.Time
}

type diagnosticEventEntry struct {
	Level      string
	OccurredAt time.Time
	Operation  string
	Message    string
}
