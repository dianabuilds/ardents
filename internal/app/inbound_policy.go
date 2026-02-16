package app

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
	PrivacyMode    MessagePrivacyMode
}

// InboundMessagePolicyDecision is the result of inbound policy evaluation.
type InboundMessagePolicyDecision struct {
	Action InboundMessagePolicyAction
	Reason InboundMessagePolicyReason
}

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

	switch NormalizePrivacySettings(PrivacySettings{MessagePrivacyMode: input.PrivacyMode}).MessagePrivacyMode {
	case MessagePrivacyRequests:
		return InboundMessagePolicyDecision{
			Action: InboundMessageActionQueueRequest,
			Reason: InboundMessageReasonUnknownMessageReq,
		}
	case MessagePrivacyEveryone:
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
