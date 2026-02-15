package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"aim-chat/go-backend/internal/app"
	"aim-chat/go-backend/internal/app/contracts"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

// SetTransportNodeForTesting swaps transport implementation and returns previous node.
// It is intended for package-level tests that need deterministic publish failures.
func (s *Service) SetTransportNodeForTesting(node contracts.TransportNode) contracts.TransportNode {
	prev := s.wakuNode
	s.wakuNode = node
	return prev
}

func (s *Service) SetPublishFailuresForTesting(failures map[string]error) {
	prev := s.wakuNode
	fail := make(map[string]error, len(failures))
	for recipient, err := range failures {
		fail[recipient] = err
	}
	s.wakuNode = &failingPublishProxy{
		base: prev,
		fail: fail,
	}
}

func (s *Service) PublishNotificationForTesting(method string, payload any) NotificationEvent {
	return s.notifier.Publish(method, payload)
}

func (s *Service) SaveMessageForTesting(msg models.Message) error {
	return s.messageStore.SaveMessage(msg)
}

func (s *Service) GetMessageForTesting(messageID string) (models.Message, bool) {
	return s.messageStore.GetMessage(messageID)
}

func (s *Service) ActiveDeviceAuthForTesting(payload []byte) (models.Device, []byte, error) {
	return s.identityManager.ActiveDeviceAuth(payload)
}

func (s *Service) BuildSignedReceiptWireForTesting(wireID, status, messageID, senderID, recipientID string) ([]byte, error) {
	wire := app.WirePayload{
		Kind: "receipt",
		Receipt: &models.MessageReceipt{
			MessageID: messageID,
			Status:    status,
			Timestamp: time.Now().UTC(),
		},
	}
	authPayload, err := app.BuildWireAuthPayload(wireID, senderID, recipientID, wire)
	if err != nil {
		return nil, err
	}
	device, sig, err := s.identityManager.ActiveDeviceAuth(authPayload)
	if err != nil {
		return nil, err
	}
	wire.Device = &device
	wire.DeviceSig = sig
	return json.Marshal(wire)
}

func (s *Service) BuildSignedPlainWireForTesting(wireID, content, senderID, recipientID string) ([]byte, error) {
	wire := app.WirePayload{
		Kind:  "plain",
		Plain: []byte(content),
	}
	authPayload, err := app.BuildWireAuthPayload(wireID, senderID, recipientID, wire)
	if err != nil {
		return nil, err
	}
	device, sig, err := s.identityManager.ActiveDeviceAuth(authPayload)
	if err != nil {
		return nil, err
	}
	wire.Device = &device
	wire.DeviceSig = sig
	return json.Marshal(wire)
}

func DeviceRevocationDeliveryStats(err error) (attempted int, failed int, full bool, ok bool) {
	var deliveryErr *contracts.DeviceRevocationDeliveryError
	if !errors.As(err, &deliveryErr) {
		return 0, 0, false, false
	}
	return deliveryErr.Attempted, deliveryErr.Failed, deliveryErr.IsFullFailure(), true
}

type failingPublishProxy struct {
	base contracts.TransportNode
	fail map[string]error
}

func (n *failingPublishProxy) Start(ctx context.Context) error {
	return n.base.Start(ctx)
}

func (n *failingPublishProxy) Stop(ctx context.Context) error {
	return n.base.Stop(ctx)
}

func (n *failingPublishProxy) Status() waku.Status {
	return n.base.Status()
}

func (n *failingPublishProxy) SetIdentity(identityID string) {
	n.base.SetIdentity(identityID)
}

func (n *failingPublishProxy) SubscribePrivate(handler func(waku.PrivateMessage)) error {
	return n.base.SubscribePrivate(handler)
}

func (n *failingPublishProxy) PublishPrivate(ctx context.Context, msg waku.PrivateMessage) error {
	if err, ok := n.fail[msg.Recipient]; ok {
		return err
	}
	return n.base.PublishPrivate(ctx, msg)
}

func (n *failingPublishProxy) FetchPrivateSince(ctx context.Context, recipient string, since time.Time, limit int) ([]waku.PrivateMessage, error) {
	return n.base.FetchPrivateSince(ctx, recipient, since, limit)
}

func (n *failingPublishProxy) ListenAddresses() []string {
	return n.base.ListenAddresses()
}

func (n *failingPublishProxy) NetworkMetrics() map[string]int {
	return n.base.NetworkMetrics()
}
