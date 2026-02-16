package app

import "testing"

func TestEvaluateInboundMessagePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  InboundMessagePolicyInput
		action InboundMessagePolicyAction
		reason InboundMessagePolicyReason
	}{
		{
			name: "blocked sender always rejected",
			input: InboundMessagePolicyInput{
				IsKnownContact: true,
				IsBlocked:      true,
				PrivacyMode:    MessagePrivacyEveryone,
			},
			action: InboundMessageActionReject,
			reason: InboundMessageReasonBlockedSender,
		},
		{
			name: "known contact bypasses privacy mode",
			input: InboundMessagePolicyInput{
				IsKnownContact: true,
				IsBlocked:      false,
				PrivacyMode:    MessagePrivacyContactsOnly,
			},
			action: InboundMessageActionAcceptChat,
			reason: InboundMessageReasonTrustedContact,
		},
		{
			name: "unknown sender rejected in contacts only",
			input: InboundMessagePolicyInput{
				IsKnownContact: false,
				IsBlocked:      false,
				PrivacyMode:    MessagePrivacyContactsOnly,
			},
			action: InboundMessageActionReject,
			reason: InboundMessageReasonUnknownContactsOnly,
		},
		{
			name: "unknown sender routed to request inbox in requests mode",
			input: InboundMessagePolicyInput{
				IsKnownContact: false,
				IsBlocked:      false,
				PrivacyMode:    MessagePrivacyRequests,
			},
			action: InboundMessageActionQueueRequest,
			reason: InboundMessageReasonUnknownMessageReq,
		},
		{
			name: "unknown sender accepted in everyone mode",
			input: InboundMessagePolicyInput{
				IsKnownContact: false,
				IsBlocked:      false,
				PrivacyMode:    MessagePrivacyEveryone,
			},
			action: InboundMessageActionAcceptChat,
			reason: InboundMessageReasonUnknownEveryoneMode,
		},
		{
			name: "invalid mode normalized to contacts only",
			input: InboundMessagePolicyInput{
				IsKnownContact: false,
				IsBlocked:      false,
				PrivacyMode:    MessagePrivacyMode("invalid"),
			},
			action: InboundMessageActionReject,
			reason: InboundMessageReasonUnknownContactsOnly,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EvaluateInboundMessagePolicy(tt.input)
			if got.Action != tt.action {
				t.Fatalf("action mismatch: got=%s want=%s", got.Action, tt.action)
			}
			if got.Reason != tt.reason {
				t.Fatalf("reason mismatch: got=%s want=%s", got.Reason, tt.reason)
			}
		})
	}
}
