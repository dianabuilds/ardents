package policy

import (
	"crypto/ed25519"
	"fmt"

	"aim-chat/go-backend/pkg/models"

	"github.com/mr-tron/base58/base58"
	"golang.org/x/crypto/blake2b"
)

var (
	ErrInvalidContactCard = fmt.Errorf("invalid contact card")
	ErrIdentityMismatch   = fmt.Errorf("identity_id does not match public key")
)

func BuildIdentityID(signingPublicKey []byte) (string, error) {
	if len(signingPublicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid signing public key size: %d", len(signingPublicKey))
	}
	h := blake2b.Sum256(signingPublicKey)
	return "aim1" + base58.Encode(h[:]), nil
}

func VerifyIdentityID(identityID string, signingPublicKey []byte) (bool, error) {
	expected, err := BuildIdentityID(signingPublicKey)
	if err != nil {
		return false, err
	}
	return identityID == expected, nil
}

func SignContactCard(identityID, displayName string, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) (models.ContactCard, error) {
	if privateKey == nil || publicKey == nil {
		return models.ContactCard{}, ErrInvalidContactCard
	}
	card := models.ContactCard{
		IdentityID:  identityID,
		DisplayName: displayName,
		PublicKey:   append([]byte(nil), publicKey...),
	}
	if ok, err := VerifyIdentityID(identityID, publicKey); err != nil || !ok {
		if err != nil {
			return models.ContactCard{}, err
		}
		return models.ContactCard{}, ErrIdentityMismatch
	}
	card.Signature = ed25519.Sign(privateKey, contactCardSigningBytes(card))
	return card, nil
}

func VerifyContactCard(card models.ContactCard) (bool, error) {
	if len(card.PublicKey) != ed25519.PublicKeySize || len(card.Signature) != ed25519.SignatureSize {
		return false, ErrInvalidContactCard
	}
	ok, err := VerifyIdentityID(card.IdentityID, card.PublicKey)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrIdentityMismatch
	}
	return ed25519.Verify(card.PublicKey, contactCardSigningBytes(card), card.Signature), nil
}

func contactCardSigningBytes(card models.ContactCard) []byte {
	b := make([]byte, 0, len(card.IdentityID)+len(card.DisplayName)+len(card.PublicKey)+2)
	b = append(b, []byte(card.IdentityID)...)
	b = append(b, 0)
	b = append(b, []byte(card.DisplayName)...)
	b = append(b, 0)
	b = append(b, card.PublicKey...)
	return b
}
