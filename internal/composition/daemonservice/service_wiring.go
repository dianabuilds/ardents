package daemonservice

import (
	"errors"

	"aim-chat/go-backend/internal/domains/contracts"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func buildMessagingDeps(svc *Service) messagingapp.ServiceDeps {
	return messagingapp.ServiceDeps{
		Identity:       svc.identityManager,
		Sessions:       svc.sessionManager,
		Messages:       svc.messageStore,
		GenerateID:     runtimeapp.GeneratePrefixedID,
		TrackOperation: svc.trackOperation,
		PublishQueued:  svc.publishQueuedMessage,
		ApplyAutoRead:  svc.applyAutoRead,
		PublishPrivate: func(msg waku.PrivateMessage) error {
			ctx, err := svc.networkContext("")
			if err != nil {
				return err
			}
			return svc.publishWithTimeout(ctx, msg)
		},
		Notify:              svc.notify,
		RecordError:         svc.recordError,
		IsMessageIDConflict: func(err error) bool { return errors.Is(err, storage.ErrMessageIDConflict) },
	}
}

func buildInboundMessagingDeps(svc *Service) messagingapp.InboundServiceDeps {
	return messagingapp.InboundServiceDeps{
		EvaluateInboundPolicy: func(senderID string) messagingapp.InboundPolicyDecision {
			decision := svc.evaluateInboundPolicy(senderID)
			switch decision.Action {
			case privacydomain.InboundMessageActionReject:
				return messagingapp.InboundPolicyDecision{
					Action: messagingapp.InboundPolicyActionReject,
					Reason: string(decision.Reason),
					Err:    privacydomain.InboundPolicyError(decision.Reason),
				}
			case privacydomain.InboundMessageActionQueueRequest:
				return messagingapp.InboundPolicyDecision{
					Action: messagingapp.InboundPolicyActionQueue,
					Reason: string(decision.Reason),
				}
			default:
				return messagingapp.InboundPolicyDecision{
					Action: messagingapp.InboundPolicyActionAccept,
					Reason: string(decision.Reason),
				}
			}
		},
		ShouldAutoAddUnknownSender: func(dec messagingapp.InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
			decision := privacydomain.InboundMessagePolicyDecision{
				Action: privacydomain.InboundMessagePolicyAction(dec.Action),
				Reason: privacydomain.InboundMessagePolicyReason(dec.Reason),
			}
			return privacydomain.ShouldAutoAddUnknownSenderContact(
				decision,
				conversationType,
				svc.identityManager.HasVerifiedContact(senderID),
				hasCard,
			)
		},
		ShouldBypassInboundDevice: func(dec messagingapp.InboundPolicyDecision, senderID, conversationType string, hasCard bool) bool {
			decision := privacydomain.InboundMessagePolicyDecision{
				Action: privacydomain.InboundMessagePolicyAction(dec.Action),
				Reason: privacydomain.InboundMessagePolicyReason(dec.Reason),
			}
			return privacydomain.ShouldBypassInboundDeviceAuth(
				decision,
				conversationType,
				svc.identityManager.HasVerifiedContact(senderID),
				hasCard,
			)
		},
		HasVerifiedContact:     svc.identityManager.HasVerifiedContact,
		AddContactByIdentityID: svc.identityManager.AddContactByIdentityID,
		ValidateInboundContactTrust: func(senderID string, wire contracts.WirePayload) *messagingapp.InboundContactTrustViolation {
			return messagingapp.ValidateInboundContactTrust(senderID, wire, svc.identityManager)
		},
		NotifySecurityAlert: svc.notifySecurityAlert,
		ApplyDeviceRevocation: func(senderID string, rev models.DeviceRevocation) error {
			return svc.identityManager.ApplyDeviceRevocation(senderID, rev)
		},
		ValidateInboundDeviceAuth: func(msg waku.PrivateMessage, wire contracts.WirePayload) error {
			return messagingapp.ValidateInboundDeviceAuth(msg, wire, svc.identityManager)
		},
		ResolveInboundContent: func(msg waku.PrivateMessage, wire contracts.WirePayload) ([]byte, string, error) {
			return messagingapp.ResolveInboundContent(msg, wire, svc.sessionManager)
		},
		HandleInboundGroupMessage: svc.handleInboundGroupMessage,
		HandleInboundGroupEvent:   svc.handleInboundGroupEvent,
		ApplyInboundReceiptStatus: svc.applyInboundReceiptStatus,
		PersistInboundMessage:     svc.persistInboundMessage,
		PersistInboundRequest:     svc.persistInboundRequest,
		SendReceiptDelivered: func(senderID, messageID string) error {
			return svc.sendReceipt(senderID, messageID, "delivered")
		},
		RecordError: svc.recordError,
	}
}
