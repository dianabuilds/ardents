package daemonservice

import (
	"errors"

	inboxapp "aim-chat/go-backend/internal/domains/inbox"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

func (s *Service) inboxUseCases() *inboxapp.Service {
	return &inboxapp.Service{
		SnapshotInbox:        s.snapshotRequestInbox,
		TakeThread:           s.takeMessageRequestThread,
		RestoreThreadIfEmpty: s.restoreMessageRequestThreadIfEmpty,
		RemoveThread:         s.removeMessageRequest,
		HasContact:           s.identityManager.HasContact,
		AddContact:           s.AddContact,
		SaveMessage:          s.messageStore.SaveMessage,
		IsMessageIDConflict:  func(err error) bool { return errors.Is(err, storage.ErrMessageIDConflict) },
		AddToBlocklist:       s.AddToBlocklist,
		RecordError:          s.recordError,
		Notify:               s.notify,
	}
}

func (s *Service) snapshotRequestInbox() map[string][]models.Message {
	s.requestRuntime.Mu.RLock()
	defer s.requestRuntime.Mu.RUnlock()
	return inboxapp.CopyInboxState(s.requestRuntime.Inbox)
}

func (s *Service) persistRequestInboxLocked() error {
	if s.requestInboxState == nil {
		return nil
	}
	return s.requestInboxState.Persist(s.requestRuntime.Inbox)
}

func (s *Service) persistRequestInboxSnapshotLocked(next map[string][]models.Message) error {
	if s.requestInboxState == nil {
		return nil
	}
	return s.requestInboxState.Persist(next)
}

func (s *Service) withRequestInboxWriteLock(fn func() error) error {
	s.requestRuntime.Mu.Lock()
	defer s.requestRuntime.Mu.Unlock()
	return fn()
}

func (s *Service) takeMessageRequestThread(senderID string) ([]models.Message, bool, error) {
	var (
		thread []models.Message
		found  bool
	)
	err := s.withRequestInboxWriteLock(func() error {
		current, exists := s.requestRuntime.Inbox[senderID]
		if !exists || len(current) == 0 {
			return nil
		}
		thread = inboxapp.CopyThread(current)
		found = true
		nextInbox := inboxapp.CopyInboxState(s.requestRuntime.Inbox)
		delete(nextInbox, senderID)
		if err := s.persistRequestInboxSnapshotLocked(nextInbox); err != nil {
			return err
		}
		s.requestRuntime.Inbox = nextInbox
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return thread, found, nil
}

func (s *Service) restoreMessageRequestThreadIfEmpty(senderID string, thread []models.Message) error {
	return s.withRequestInboxWriteLock(func() error {
		if len(s.requestRuntime.Inbox[senderID]) > 0 {
			return nil
		}
		nextInbox := inboxapp.CopyInboxState(s.requestRuntime.Inbox)
		nextInbox[senderID] = inboxapp.CopyThread(thread)
		if err := s.persistRequestInboxSnapshotLocked(nextInbox); err != nil {
			return err
		}
		s.requestRuntime.Inbox = nextInbox
		return nil
	})
}

func (s *Service) removeMessageRequest(senderID string) (bool, error) {
	var removed bool
	err := s.withRequestInboxWriteLock(func() error {
		if _, exists := s.requestRuntime.Inbox[senderID]; !exists {
			return nil
		}
		nextInbox := inboxapp.CopyInboxState(s.requestRuntime.Inbox)
		delete(nextInbox, senderID)
		if err := s.persistRequestInboxSnapshotLocked(nextInbox); err != nil {
			return err
		}
		s.requestRuntime.Inbox = nextInbox
		removed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return removed, nil
}
