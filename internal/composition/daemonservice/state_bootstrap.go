package daemonservice

import (
	"time"

	daemoncomposition "aim-chat/go-backend/internal/composition/daemon"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	inboxapp "aim-chat/go-backend/internal/domains/inbox"
	"aim-chat/go-backend/pkg/models"
)

func (s *Service) bootstrapStateStores(bundle daemoncomposition.StorageBundle, secret string) {
	s.requestInboxState.Configure(bundle.RequestInboxPath, secret)
	inbox, err := s.requestInboxState.Bootstrap()
	if err != nil {
		s.logger.Warn("message request inbox bootstrap failed, using empty list", "error", err.Error())
		inbox = map[string][]models.Message{}
	}
	s.requestRuntime.SetInbox(inboxapp.CopyInboxState(inbox))

	s.groupStateStore.Configure(bundle.GroupStatePath, secret)
	groupStates, groupEventLog, err := s.groupStateStore.Bootstrap()
	if err != nil {
		s.logger.Warn("group state bootstrap failed, using empty state", "error", err.Error())
		groupStates = map[string]groupdomain.GroupState{}
		groupEventLog = map[string][]groupdomain.GroupEvent{}
	}
	s.groupRuntime.SetSnapshot(groupStates, groupEventLog)
	if s.groupRuntime.ReplaySeen == nil {
		s.groupRuntime.ReplaySeen = make(map[string]time.Time)
	}

	s.bindingStore.Configure(bundle.NodeBindingPath, secret)
	if err := s.bindingStore.Bootstrap(); err != nil {
		s.logger.Warn("node binding bootstrap failed, using empty state", "error", err.Error())
	}
}
