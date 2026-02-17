package privacy

import "testing"

func TestEvaluateInboundMessagePolicy(t *testing.T) {
	runInboundPolicyCases(
		t,
		func(tc inboundPolicyCase) InboundMessagePolicyDecision {
			return EvaluateInboundMessagePolicy(InboundMessagePolicyInput{
				IsKnownContact: tc.isKnownContact,
				IsBlocked:      tc.isBlocked,
				PrivacyMode:    tc.mode,
			})
		},
		func(out InboundMessagePolicyDecision) string { return string(out.Action) },
		func(out InboundMessagePolicyDecision) string { return string(out.Reason) },
		func(tc inboundPolicyCase) string { return string(tc.messageAction) },
		func(tc inboundPolicyCase) string { return string(tc.messageReason) },
	)
}
