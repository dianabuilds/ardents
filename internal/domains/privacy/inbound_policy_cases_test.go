package privacy

import "testing"

type inboundPolicyCase struct {
	name string
	mode MessagePrivacyMode

	isKnownContact bool
	isBlocked      bool

	messageAction InboundMessagePolicyAction
	messageReason InboundMessagePolicyReason

	inviteAction InboundGroupInvitePolicyAction
	inviteReason InboundGroupInvitePolicyReason
}

func inboundPolicyCases() []inboundPolicyCase {
	return []inboundPolicyCase{
		{
			name:           "blocked sender always rejected",
			isKnownContact: true,
			isBlocked:      true,
			mode:           MessagePrivacyEveryone,
			messageAction:  InboundMessageActionReject,
			messageReason:  InboundMessageReasonBlockedSender,
			inviteAction:   InboundGroupInviteActionReject,
			inviteReason:   InboundGroupInviteReasonBlockedSender,
		},
		{
			name:           "known contact bypasses privacy mode",
			isKnownContact: true,
			isBlocked:      false,
			mode:           MessagePrivacyContactsOnly,
			messageAction:  InboundMessageActionAcceptChat,
			messageReason:  InboundMessageReasonTrustedContact,
			inviteAction:   InboundGroupInviteActionAcceptInvite,
			inviteReason:   InboundGroupInviteReasonTrustedContact,
		},
		{
			name:           "unknown sender rejected in contacts only",
			isKnownContact: false,
			isBlocked:      false,
			mode:           MessagePrivacyContactsOnly,
			messageAction:  InboundMessageActionReject,
			messageReason:  InboundMessageReasonUnknownContactsOnly,
			inviteAction:   InboundGroupInviteActionReject,
			inviteReason:   InboundGroupInviteReasonUnknownContactsOnly,
		},
		{
			name:           "unknown sender routed to request inbox in requests mode",
			isKnownContact: false,
			isBlocked:      false,
			mode:           MessagePrivacyRequests,
			messageAction:  InboundMessageActionQueueRequest,
			messageReason:  InboundMessageReasonUnknownMessageReq,
			inviteAction:   InboundGroupInviteActionQueueRequest,
			inviteReason:   InboundGroupInviteReasonUnknownMessageReq,
		},
		{
			name:           "unknown sender accepted in everyone mode",
			isKnownContact: false,
			isBlocked:      false,
			mode:           MessagePrivacyEveryone,
			messageAction:  InboundMessageActionAcceptChat,
			messageReason:  InboundMessageReasonUnknownEveryoneMode,
			inviteAction:   InboundGroupInviteActionAcceptInvite,
			inviteReason:   InboundGroupInviteReasonUnknownEveryoneMode,
		},
		{
			name:           "invalid mode normalized to everyone",
			isKnownContact: false,
			isBlocked:      false,
			mode:           "invalid",
			messageAction:  InboundMessageActionAcceptChat,
			messageReason:  InboundMessageReasonUnknownEveryoneMode,
			inviteAction:   InboundGroupInviteActionAcceptInvite,
			inviteReason:   InboundGroupInviteReasonUnknownEveryoneMode,
		},
	}
}

func runInboundPolicyCases[T any](
	t *testing.T,
	evaluate func(inboundPolicyCase) T,
	actualAction func(T) string,
	actualReason func(T) string,
	expectedAction func(inboundPolicyCase) string,
	expectedReason func(inboundPolicyCase) string,
) {
	t.Helper()
	t.Parallel()
	for _, tc := range inboundPolicyCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := evaluate(tc)
			if actualAction(got) != expectedAction(tc) {
				t.Fatalf("action mismatch: got=%s want=%s", actualAction(got), expectedAction(tc))
			}
			if actualReason(got) != expectedReason(tc) {
				t.Fatalf("reason mismatch: got=%s want=%s", actualReason(got), expectedReason(tc))
			}
		})
	}
}
