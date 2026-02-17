package messaging

import (
	"aim-chat/go-backend/internal/domains/contracts"
	"errors"
	"testing"

	"aim-chat/go-backend/pkg/models"
)

type fakeInboundTrustIdentity struct {
	verified      bool
	pinnedKey     []byte
	hasPinnedKey  bool
	verifyCardOK  bool
	verifyCardErr error
	addContactErr error
}

func (f *fakeInboundTrustIdentity) HasVerifiedContact(_ string) bool {
	return f.verified
}

func (f *fakeInboundTrustIdentity) VerifyContactCard(_ models.ContactCard) (bool, error) {
	if f.verifyCardErr != nil {
		return false, f.verifyCardErr
	}
	return f.verifyCardOK, nil
}

func (f *fakeInboundTrustIdentity) ContactPublicKey(_ string) ([]byte, bool) {
	return f.pinnedKey, f.hasPinnedKey
}

func (f *fakeInboundTrustIdentity) AddContact(_ models.ContactCard) error {
	return f.addContactErr
}

func TestValidateInboundContactTrust_UnverifiedMissingCard(t *testing.T) {
	id := &fakeInboundTrustIdentity{verified: false}
	viol := ValidateInboundContactTrust("sender-1", contracts.WirePayload{}, id)
	if viol == nil || viol.AlertCode != "unverified_sender_missing_card" {
		t.Fatalf("expected unverified_sender_missing_card, got %#v", viol)
	}
}

func TestValidateInboundContactTrust_VerifiedPinMismatch(t *testing.T) {
	id := &fakeInboundTrustIdentity{
		verified:     true,
		verifyCardOK: true,
		hasPinnedKey: true,
		pinnedKey:    []byte("old-key"),
	}
	wire := contracts.WirePayload{
		Card: &models.ContactCard{
			IdentityID: "sender-1",
			PublicKey:  []byte("new-key"),
		},
	}
	viol := ValidateInboundContactTrust("sender-1", wire, id)
	if viol == nil || viol.AlertCode != "contact_key_pin_mismatch" {
		t.Fatalf("expected contact_key_pin_mismatch, got %#v", viol)
	}
}

func TestValidateInboundContactTrust_VerificationError(t *testing.T) {
	id := &fakeInboundTrustIdentity{
		verified:      true,
		verifyCardErr: errors.New("bad card"),
	}
	wire := contracts.WirePayload{
		Card: &models.ContactCard{
			IdentityID: "sender-1",
			PublicKey:  []byte("key"),
		},
	}
	viol := ValidateInboundContactTrust("sender-1", wire, id)
	if viol == nil || viol.AlertCode != "contact_card_verification_failed" {
		t.Fatalf("expected contact_card_verification_failed, got %#v", viol)
	}
}

func TestValidateInboundContactTrust_Ok(t *testing.T) {
	id := &fakeInboundTrustIdentity{
		verified:     true,
		verifyCardOK: true,
		hasPinnedKey: true,
		pinnedKey:    []byte("key"),
	}
	wire := contracts.WirePayload{
		Card: &models.ContactCard{
			IdentityID: "sender-1",
			PublicKey:  []byte("key"),
		},
	}
	viol := ValidateInboundContactTrust("sender-1", wire, id)
	if viol != nil {
		t.Fatalf("expected no violation, got %#v", viol)
	}
}
