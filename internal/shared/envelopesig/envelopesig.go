package envelopesig

import (
	"crypto/ed25519"

	"github.com/dianabuilds/ardents/internal/shared/ids"
)

func Sign(priv ed25519.PrivateKey, signingBytes func() ([]byte, error)) ([]byte, error) {
	b, err := signingBytes()
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(priv, b), nil
}

func Verify(pub ed25519.PublicKey, sig []byte, signingBytes func() ([]byte, error)) (bool, error) {
	b, err := signingBytes()
	if err != nil {
		return false, err
	}
	return ed25519.Verify(pub, b, sig), nil
}

func VerifyIdentity(identityID string, sig []byte, signingBytes func() ([]byte, error), errEmptyID error, errMissingSig error, errInvalidSig error) error {
	if identityID == "" {
		return errEmptyID
	}
	pub, err := ids.IdentityPublicKey(identityID)
	if err != nil {
		return err
	}
	if len(sig) == 0 {
		return errMissingSig
	}
	ok, err := Verify(pub, sig, signingBytes)
	if err != nil {
		return err
	}
	if !ok {
		return errInvalidSig
	}
	return nil
}
