package relay

import (
	"crypto/ed25519"
	"errors"

	"github.com/dianabuilds/ardents/internal/shared/cryptobox"
)

var (
	ErrDecryptFailed = errors.New("ERR_RELAY_DECRYPT_FAILED")
)

func SealInner(recipient ed25519.PublicKey, inner []byte) ([]byte, error) {
	return cryptobox.SealAnonymous(nil, recipient, inner)
}

func OpenInner(recipientPub ed25519.PublicKey, recipientPriv ed25519.PrivateKey, sealed []byte) ([]byte, error) {
	out, err := cryptobox.OpenAnonymous(recipientPub, recipientPriv, sealed)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return out, nil
}
