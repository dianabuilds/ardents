package cryptobox

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"errors"
	"io"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/nacl/box"
)

var ErrKeyConversion = errors.New("ERR_KEY_CONVERSION")

func Ed25519PublicKeyToX25519(pub ed25519.PublicKey) ([32]byte, error) {
	var out [32]byte
	if len(pub) != ed25519.PublicKeySize {
		return out, ErrKeyConversion
	}
	var edPub [32]byte
	copy(edPub[:], pub[:32])
	point, err := new(edwards25519.Point).SetBytes(edPub[:])
	if err != nil {
		return out, ErrKeyConversion
	}
	mb := point.BytesMontgomery()
	copy(out[:], mb)
	return out, nil
}

func Ed25519PrivateKeyToX25519(priv ed25519.PrivateKey) ([32]byte, error) {
	var out [32]byte
	if len(priv) != ed25519.PrivateKeySize {
		return out, ErrKeyConversion
	}
	seed := priv.Seed()
	h := sha512.Sum512(seed)
	copy(out[:], h[:32])
	out[0] &= 248  // #nosec G602 -- out is fixed-size [32]byte; indices are safe.
	out[31] &= 127 // #nosec G602 -- out is fixed-size [32]byte; indices are safe.
	out[31] |= 64  // #nosec G602 -- out is fixed-size [32]byte; indices are safe.
	return out, nil
}

func SealAnonymous(randSource io.Reader, recipient ed25519.PublicKey, msg []byte) ([]byte, error) {
	if randSource == nil {
		randSource = rand.Reader
	}
	xpub, err := Ed25519PublicKeyToX25519(recipient)
	if err != nil {
		return nil, err
	}
	return box.SealAnonymous(nil, msg, &xpub, randSource)
}

func OpenAnonymous(recipientPub ed25519.PublicKey, recipientPriv ed25519.PrivateKey, sealed []byte) ([]byte, error) {
	xpub, err := Ed25519PublicKeyToX25519(recipientPub)
	if err != nil {
		return nil, err
	}
	xpriv, err := Ed25519PrivateKeyToX25519(recipientPriv)
	if err != nil {
		return nil, err
	}
	out, ok := box.OpenAnonymous(nil, sealed, &xpub, &xpriv)
	if !ok {
		return nil, ErrKeyConversion
	}
	return out, nil
}
