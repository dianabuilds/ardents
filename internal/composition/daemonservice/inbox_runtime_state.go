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
		delete(s.requestRuntime.Inbox, senderID)
		if err := s.persistRequestInboxLocked(); err != nil {
			s.requestRuntime.Inbox[senderID] = thread
			return err
		}
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
		s.requestRuntime.Inbox[senderID] = inboxapp.CopyThread(thread)
		return s.persistRequestInboxLocked()
	})
}

func (s *Service) removeMessageRequest(senderID string) (bool, error) {
	var removed bool
	err := s.withRequestInboxWriteLock(func() error {
		if _, exists := s.requestRuntime.Inbox[senderID]; !exists {
			return nil
		}
		delete(s.requestRuntime.Inbox, senderID)
		if err := s.persistRequestInboxLocked(); err != nil {
			return err
		}
		removed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return removed, nil
}
