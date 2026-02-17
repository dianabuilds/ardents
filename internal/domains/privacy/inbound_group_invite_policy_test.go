package privacy

import "testing"

func TestEvaluateInboundGroupInvitePolicy(t *testing.T) {
	runInboundPolicyCases(
		t,
		func(tc inboundPolicyCase) InboundGroupInvitePolicyDecision {
			return EvaluateInboundGroupInvitePolicy(InboundGroupInvitePolicyInput{
				IsKnownContact: tc.isKnownContact,
				IsBlocked:      tc.isBlocked,
				PrivacyMode:    tc.mode,
			})
		},
		func(out InboundGroupInvitePolicyDecision) string { return string(out.Action) },
		func(out InboundGroupInvitePolicyDecision) string { return string(out.Reason) },
		func(tc inboundPolicyCase) string { return string(tc.inviteAction) },
		func(tc inboundPolicyCase) string { return string(tc.inviteReason) },
	)
}
