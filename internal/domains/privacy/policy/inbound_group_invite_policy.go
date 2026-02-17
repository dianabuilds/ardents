package policy

import privacymodel "aim-chat/go-backend/internal/domains/privacy/model"

// InboundGroupInvitePolicyAction defines how inbound group invite should be routed.
type InboundGroupInvitePolicyAction string

const (
	InboundGroupInviteActionReject       InboundGroupInvitePolicyAction = "reject"
	InboundGroupInviteActionAcceptInvite InboundGroupInvitePolicyAction = "accept_invite"
	InboundGroupInviteActionQueueRequest InboundGroupInvitePolicyAction = "queue_request"
)

// InboundGroupInvitePolicyReason explains why invite routing decision was made.
type InboundGroupInvitePolicyReason string

const (
	InboundGroupInviteReasonTrustedContact      InboundGroupInvitePolicyReason = "trusted_contact"
	InboundGroupInviteReasonBlockedSender       InboundGroupInvitePolicyReason = "blocked_sender"
	InboundGroupInviteReasonUnknownContactsOnly InboundGroupInvitePolicyReason = "unknown_sender_contacts_only"
	InboundGroupInviteReasonUnknownMessageReq   InboundGroupInvitePolicyReason = "unknown_sender_requests_mode"
	InboundGroupInviteReasonUnknownEveryoneMode InboundGroupInvitePolicyReason = "unknown_sender_everyone_mode"
)

// InboundGroupInvitePolicyInput includes data required to evaluate invite access.
type InboundGroupInvitePolicyInput struct {
	IsKnownContact bool
	IsBlocked      bool
	PrivacyMode    privacymodel.MessagePrivacyMode
}

// InboundGroupInvitePolicyDecision is the result of invite policy evaluation.
type InboundGroupInvitePolicyDecision struct {
	Action InboundGroupInvitePolicyAction
	Reason InboundGroupInvitePolicyReason
}

// EvaluateInboundGroupInvitePolicy applies invite policy gate:
// blocklist > known contact > privacy mode for unknown sender.
func EvaluateInboundGroupInvitePolicy(input InboundGroupInvitePolicyInput) InboundGroupInvitePolicyDecision {
	base := EvaluateInboundMessagePolicy(InboundMessagePolicyInput(input))

	switch base.Action {
	case InboundMessageActionReject:
		switch base.Reason {
		case InboundMessageReasonBlockedSender:
			return InboundGroupInvitePolicyDecision{
				Action: InboundGroupInviteActionReject,
				Reason: InboundGroupInviteReasonBlockedSender,
			}
		default:
			return InboundGroupInvitePolicyDecision{
				Action: InboundGroupInviteActionReject,
				Reason: InboundGroupInviteReasonUnknownContactsOnly,
			}
		}
	case InboundMessageActionQueueRequest:
		return InboundGroupInvitePolicyDecision{
			Action: InboundGroupInviteActionQueueRequest,
			Reason: InboundGroupInviteReasonUnknownMessageReq,
		}
	default:
		if base.Reason == InboundMessageReasonTrustedContact {
			return InboundGroupInvitePolicyDecision{
				Action: InboundGroupInviteActionAcceptInvite,
				Reason: InboundGroupInviteReasonTrustedContact,
			}
		}
		return InboundGroupInvitePolicyDecision{
			Action: InboundGroupInviteActionAcceptInvite,
			Reason: InboundGroupInviteReasonUnknownEveryoneMode,
		}
	}
}
