package daemonservice

import (
	"aim-chat/go-backend/internal/domains/contracts"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/pkg/models"
	"errors"
	"time"
)

func (s *Service) buildStoredMessageWire(msg models.Message) (contracts.WirePayload, error) {
	return s.messagingCore.BuildStoredMessageWire(msg)
}

func (s *Service) sendReceipt(contactID, messageID, status string) error {
	if !s.identityManager.HasVerifiedContact(contactID) {
		return errors.New("receipt target is not a verified contact")
	}
	wire := messagingapp.NewReceiptWire(messageID, status, time.Now())
	wireID, err := runtimeapp.GeneratePrefixedID("rcpt")
	if err != nil {
		return err
	}
	ctx, err := s.networkContext("network")
	if err != nil {
		return err
	}
	return s.publishSignedWireWithContext(ctx, wireID, contactID, wire)
}

func (s *Service) applyAutoRead(message *models.Message, contactID string) {
	if message == nil {
		return
	}
	if !s.updateMessageStatusAndNotify(message.ID, "read") {
		return
	}
	message.Status = "read"
	if err := s.sendReceipt(contactID, message.ID, "read"); err != nil {
		s.recordError("network", err)
	}
}

func (s *Service) publishQueuedMessage(msg models.Message, contactID string, wire contracts.WirePayload) (string, error) {
	s.logger.Info("message queued", "message_id", msg.ID, "contact_id", contactID, "kind", wire.Kind)
	ctx, err := s.networkContext("network")
	if err == nil {
		err = s.publishSignedWireWithContext(ctx, msg.ID, contactID, wire)
	}
	if err != nil {
		category := messagingapp.ErrorCategory(err)
		s.recordError(category, err)
		if category == "network" {
			if perr := s.messageStore.AddOrUpdatePending(msg, 1, messagingapp.NextRetryTime(1), err.Error()); perr != nil {
				s.recordError("storage", perr)
				return "", perr
			}
			return msg.ID, nil
		}
		return "", err
	}
	s.logger.Info("message published", "message_id", msg.ID, "contact_id", contactID)
	s.markMessageAsSent(msg.ID)
	return msg.ID, nil
}
