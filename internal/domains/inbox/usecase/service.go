package usecase

import (
	inboxmodel "aim-chat/go-backend/internal/domains/inbox/model"
	messagingdomain "aim-chat/go-backend/internal/domains/messaging"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
	"errors"
)

type Service struct {
	SnapshotInbox        func() map[string][]models.Message
	TakeThread           func(senderID string) ([]models.Message, bool, error)
	RestoreThreadIfEmpty func(senderID string, thread []models.Message) error
	RemoveThread         func(senderID string) (bool, error)
	HasContact           func(senderID string) bool
	AddContact           func(contactID, displayName string) error
	SaveMessage          func(msg models.Message) error
	AddToBlocklist       func(senderID string) ([]string, error)
	RecordError          func(category string, err error)
	Notify               func(method string, payload any)
}

func (s *Service) recordStorageError(err error) {
	if s.RecordError != nil {
		s.RecordError("storage", err)
	}
}

func (s *Service) removeThreadWithStorageError(senderID string) (bool, error) {
	exists, err := s.RemoveThread(senderID)
	if err != nil {
		s.recordStorageError(err)
	}
	return exists, err
}

func (s *Service) notifyThreadRemoved(senderID, method string) {
	if s.Notify == nil {
		return
	}
	s.Notify(method, map[string]any{
		"contact_id": senderID,
	})
}

func (s *Service) removeRequestThreadAndNotify(senderID, method string) (bool, error) {
	removed, err := s.removeThreadWithStorageError(senderID)
	if err != nil {
		return false, err
	}
	if removed {
		s.notifyThreadRemoved(senderID, method)
	}
	return removed, nil
}

func (s *Service) ListMessageRequests() ([]models.MessageRequest, error) {
	inbox := s.SnapshotInbox()
	out := make([]models.MessageRequest, 0, len(inbox))
	for senderID, messages := range inbox {
		summary, err := inboxmodel.BuildMessageRequestSummary(senderID, messages)
		if err != nil {
			continue
		}
		out = append(out, summary)
	}
	inboxmodel.SortMessageRequestsByRecency(out)
	return out, nil
}

func (s *Service) GetMessageRequest(senderID string) (models.MessageRequestThread, error) {
	senderID, err := messagingdomain.ValidateListMessagesContactID(senderID)
	if err != nil {
		return models.MessageRequestThread{}, err
	}

	inbox := s.SnapshotInbox()
	thread, ok := inbox[senderID]
	if !ok || len(thread) == 0 {
		return models.MessageRequestThread{}, inboxmodel.ErrMessageRequestNotFound
	}

	summary, err := inboxmodel.BuildMessageRequestSummary(senderID, thread)
	if err != nil {
		return models.MessageRequestThread{}, err
	}
	return models.MessageRequestThread{
		Request:  summary,
		Messages: inboxmodel.CloneMessages(thread),
	}, nil
}

func (s *Service) AcceptMessageRequest(senderID string) (bool, error) {
	senderID, err := messagingdomain.ValidateListMessagesContactID(senderID)
	if err != nil {
		return false, err
	}

	thread, exists, err := s.TakeThread(senderID)
	if err != nil {
		s.recordStorageError(err)
		return false, err
	}
	if !exists || len(thread) == 0 {
		if s.HasContact != nil && s.HasContact(senderID) {
			return true, nil
		}
		return false, inboxmodel.ErrMessageRequestNotFound
	}

	if s.AddContact != nil {
		if err := s.AddContact(senderID, senderID); err != nil {
			if s.RestoreThreadIfEmpty != nil {
				if perr := s.RestoreThreadIfEmpty(senderID, thread); perr != nil {
					s.recordStorageError(perr)
					return false, errors.Join(err, perr)
				}
			}
			return false, err
		}
	}

	for _, msg := range thread {
		if s.SaveMessage == nil {
			continue
		}
		if err := s.SaveMessage(msg); err != nil {
			if errors.Is(err, storage.ErrMessageIDConflict) {
				continue
			}
			s.recordStorageError(err)
			return false, err
		}
		if s.Notify != nil {
			s.Notify("notify.message.new", map[string]any{
				"contact_id": senderID,
				"message":    msg,
			})
		}
	}
	if s.Notify != nil {
		s.Notify("notify.request.accepted", map[string]any{
			"contact_id": senderID,
			"moved":      len(thread),
		})
	}
	return true, nil
}

func (s *Service) DeclineMessageRequest(senderID string) (bool, error) {
	senderID, err := messagingdomain.ValidateListMessagesContactID(senderID)
	if err != nil {
		return false, err
	}
	_, err = s.removeRequestThreadAndNotify(senderID, "notify.request.declined")
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) BlockSender(senderID string) (models.BlockSenderResult, error) {
	senderID, err := privacydomain.NormalizeIdentityID(senderID)
	if err != nil {
		return models.BlockSenderResult{}, err
	}
	contactExists := s.HasContact != nil && s.HasContact(senderID)
	if s.AddToBlocklist == nil {
		return models.BlockSenderResult{}, errors.New("blocklist operation is not configured")
	}
	blocked, err := s.AddToBlocklist(senderID)
	if err != nil {
		return models.BlockSenderResult{}, err
	}
	requestRemoved, err := s.removeRequestThreadAndNotify(senderID, "notify.request.blocked")
	if err != nil {
		return models.BlockSenderResult{}, err
	}
	if s.Notify != nil {
		s.Notify("notify.sender.blocked", map[string]any{
			"contact_id":      senderID,
			"request_removed": requestRemoved,
			"contact_exists":  contactExists,
		})
	}
	return models.BlockSenderResult{
		Blocked:        blocked,
		RequestRemoved: requestRemoved,
		ContactExists:  contactExists,
	}, nil
}
