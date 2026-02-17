package daemonservice

import (
	"log/slog"
	"sync"

	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	identityapp "aim-chat/go-backend/internal/domains/identity"
	inboxapp "aim-chat/go-backend/internal/domains/inbox"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	privacyapp "aim-chat/go-backend/internal/domains/privacy"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
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
	ListGroupMembers(groupID string) ([]groupdomain.GroupMember, error)
	LeaveGroup(groupID string) (bool, error)
	InviteToGroup(groupID, memberID string) (groupdomain.GroupMember, error)
	AcceptGroupInvite(groupID string) (bool, error)
	DeclineGroupInvite(groupID string) (bool, error)
	RemoveGroupMember(groupID, memberID string) (bool, error)
	PromoteGroupMember(groupID, memberID string) (groupdomain.GroupMember, error)
	DemoteGroupMember(groupID, memberID string) (groupdomain.GroupMember, error)
	SendGroupMessage(groupID, content string) (groupdomain.GroupMessageFanoutResult, error)
	ListGroupMessages(groupID string, limit, offset int) ([]models.Message, error)
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
	metrics           *runtimeapp.ServiceMetricsState
	runtime           *runtimeapp.ServiceRuntime
	requestRuntime    *inboxapp.RuntimeState
	groupRuntime      *groupdomain.RuntimeState
	identityState     *identityapp.StateStore
	privacyState      *privacyapp.SettingsStore
	requestInboxState *inboxapp.RequestStore
	groupStateStore   *groupdomain.SnapshotStore
	groupAbuse        *groupdomain.AbuseProtection
	startStopMu       *sync.Mutex
}
