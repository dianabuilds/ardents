package policy

import (
	"errors"
	"strings"

	"aim-chat/go-backend/pkg/models"
)

import privacymodel "aim-chat/go-backend/internal/domains/privacy/model"

// InboundMessagePolicyAction defines how an inbound message should be routed.
type InboundMessagePolicyAction string

const (
	InboundMessageActionReject       InboundMessagePolicyAction = "reject"
	InboundMessageActionAcceptChat   InboundMessagePolicyAction = "accept_chat"
	InboundMessageActionQueueRequest InboundMessagePolicyAction = "queue_request"
)

// InboundMessagePolicyReason describes why the policy returned a specific action.
type InboundMessagePolicyReason string

const (
	InboundMessageReasonTrustedContact      InboundMessagePolicyReason = "trusted_contact"
	InboundMessageReasonBlockedSender       InboundMessagePolicyReason = "blocked_sender"
	InboundMessageReasonUnknownContactsOnly InboundMessagePolicyReason = "unknown_sender_contacts_only"
	InboundMessageReasonUnknownMessageReq   InboundMessagePolicyReason = "unknown_sender_requests_mode"
	InboundMessageReasonUnknownEveryoneMode InboundMessagePolicyReason = "unknown_sender_everyone_mode"
)

// InboundMessagePolicyInput contains all fields required to decide inbound access.
type InboundMessagePolicyInput struct {
	IsKnownContact bool
	IsBlocked      bool
	PrivacyMode    privacymodel.MessagePrivacyMode
}

// InboundMessagePolicyDecision is the result of inbound policy evaluation.
type InboundMessagePolicyDecision struct {
	Action InboundMessagePolicyAction
	Reason InboundMessagePolicyReason
}

var ErrInboundBlockedSender = errors.New("sender is blocked")
var ErrInboundUnknownContactsOnly = errors.New("sender is not an added contact")
var ErrInboundUnknownMessageReq = errors.New("sender must be accepted through message requests")
var ErrInboundRejected = errors.New("inbound message rejected by policy")

// EvaluateInboundMessagePolicy applies unified inbound policy:
// blocklist > known contact > privacy mode for unknown sender.
func EvaluateInboundMessagePolicy(input InboundMessagePolicyInput) InboundMessagePolicyDecision {
	if input.IsBlocked {
		return InboundMessagePolicyDecision{
			Action: InboundMessageActionReject,
			Reason: InboundMessageReasonBlockedSender,
		}
	}
	if input.IsKnownContact {
		return InboundMessagePolicyDecision{
			Action: InboundMessageActionAcceptChat,
			Reason: InboundMessageReasonTrustedContact,
		}
	}

	switch privacymodel.NormalizePrivacySettings(privacymodel.PrivacySettings{MessagePrivacyMode: input.PrivacyMode}).MessagePrivacyMode {
	case privacymodel.MessagePrivacyRequests:
		return InboundMessagePolicyDecision{
			Action: InboundMessageActionQueueRequest,
			Reason: InboundMessageReasonUnknownMessageReq,
		}
	case privacymodel.MessagePrivacyEveryone:
		return InboundMessagePolicyDecision{
			Action: InboundMessageActionAcceptChat,
			Reason: InboundMessageReasonUnknownEveryoneMode,
		}
	default:
		return InboundMessagePolicyDecision{
			Action: InboundMessageActionReject,
			Reason: InboundMessageReasonUnknownContactsOnly,
		}
	}
}

// ShouldAutoAddUnknownSenderContact returns true when unknown sender in everyone mode
// can be auto-added as contact without trusting card-bound auth.
func ShouldAutoAddUnknownSenderContact(
	decision InboundMessagePolicyDecision,
	conversationType string,
	isVerifiedContact bool,
	hasCard bool,
) bool {
	if strings.TrimSpace(conversationType) == models.ConversationTypeGroup {
		return false
	}
	if decision.Reason != InboundMessageReasonUnknownEveryoneMode {
		return false
	}
	if isVerifiedContact {
		return false
	}
	return !hasCard
}

// ShouldBypassInboundDeviceAuth returns true when unknown sender in everyone mode
// may bypass strict inbound device auth for card-less messages.
func ShouldBypassInboundDeviceAuth(
	decision InboundMessagePolicyDecision,
	conversationType string,
	isVerifiedContact bool,
	hasCard bool,
) bool {
	if strings.TrimSpace(conversationType) == models.ConversationTypeGroup {
		return false
	}
	if decision.Reason != InboundMessageReasonUnknownEveryoneMode {
		return false
	}
	if isVerifiedContact {
		return false
	}
	return !hasCard
}

func InboundPolicyError(reason InboundMessagePolicyReason) error {
	switch reason {
	case InboundMessageReasonBlockedSender:
		return ErrInboundBlockedSender
	case InboundMessageReasonUnknownContactsOnly:
		return ErrInboundUnknownContactsOnly
	case InboundMessageReasonUnknownMessageReq:
		return ErrInboundUnknownMessageReq
	default:
		return ErrInboundRejected
	}
}
