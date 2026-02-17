package privacy

import (
	privacypolicy "aim-chat/go-backend/internal/domains/privacy/policy"
)

type InboundMessagePolicyAction = privacypolicy.InboundMessagePolicyAction
type InboundMessagePolicyReason = privacypolicy.InboundMessagePolicyReason
type InboundMessagePolicyInput = privacypolicy.InboundMessagePolicyInput
type InboundMessagePolicyDecision = privacypolicy.InboundMessagePolicyDecision

const (
	InboundMessageActionReject       = privacypolicy.InboundMessageActionReject
	InboundMessageActionAcceptChat   = privacypolicy.InboundMessageActionAcceptChat
	InboundMessageActionQueueRequest = privacypolicy.InboundMessageActionQueueRequest
)

const (
	InboundMessageReasonTrustedContact      = privacypolicy.InboundMessageReasonTrustedContact
	InboundMessageReasonBlockedSender       = privacypolicy.InboundMessageReasonBlockedSender
	InboundMessageReasonUnknownContactsOnly = privacypolicy.InboundMessageReasonUnknownContactsOnly
	InboundMessageReasonUnknownMessageReq   = privacypolicy.InboundMessageReasonUnknownMessageReq
	InboundMessageReasonUnknownEveryoneMode = privacypolicy.InboundMessageReasonUnknownEveryoneMode
)

func EvaluateInboundMessagePolicy(input InboundMessagePolicyInput) InboundMessagePolicyDecision {
	return privacypolicy.EvaluateInboundMessagePolicy(input)
}

func ShouldAutoAddUnknownSenderContact(
	decision InboundMessagePolicyDecision,
	conversationType string,
	isVerifiedContact bool,
	hasCard bool,
) bool {
	return privacypolicy.ShouldAutoAddUnknownSenderContact(decision, conversationType, isVerifiedContact, hasCard)
}

func ShouldBypassInboundDeviceAuth(
	decision InboundMessagePolicyDecision,
	conversationType string,
	isVerifiedContact bool,
	hasCard bool,
) bool {
	return privacypolicy.ShouldBypassInboundDeviceAuth(decision, conversationType, isVerifiedContact, hasCard)
}

func InboundPolicyError(reason InboundMessagePolicyReason) error {
	return privacypolicy.InboundPolicyError(reason)
}

type InboundGroupInvitePolicyAction = privacypolicy.InboundGroupInvitePolicyAction
type InboundGroupInvitePolicyReason = privacypolicy.InboundGroupInvitePolicyReason
type InboundGroupInvitePolicyInput = privacypolicy.InboundGroupInvitePolicyInput
type InboundGroupInvitePolicyDecision = privacypolicy.InboundGroupInvitePolicyDecision

const (
	InboundGroupInviteActionReject       = privacypolicy.InboundGroupInviteActionReject
	InboundGroupInviteActionAcceptInvite = privacypolicy.InboundGroupInviteActionAcceptInvite
	InboundGroupInviteActionQueueRequest = privacypolicy.InboundGroupInviteActionQueueRequest
)

const (
	InboundGroupInviteReasonTrustedContact      = privacypolicy.InboundGroupInviteReasonTrustedContact
	InboundGroupInviteReasonBlockedSender       = privacypolicy.InboundGroupInviteReasonBlockedSender
	InboundGroupInviteReasonUnknownContactsOnly = privacypolicy.InboundGroupInviteReasonUnknownContactsOnly
	InboundGroupInviteReasonUnknownMessageReq   = privacypolicy.InboundGroupInviteReasonUnknownMessageReq
	InboundGroupInviteReasonUnknownEveryoneMode = privacypolicy.InboundGroupInviteReasonUnknownEveryoneMode
)

func EvaluateInboundGroupInvitePolicy(input InboundGroupInvitePolicyInput) InboundGroupInvitePolicyDecision {
	return privacypolicy.EvaluateInboundGroupInvitePolicy(input)
}
