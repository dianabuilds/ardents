package ed25519util

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

var ErrPrivateKeyInvalid = errors.New("ERR_PRIVATE_KEY_INVALID")

func ParsePrivateKeyPEM(keyPEM []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, ErrPrivateKeyInvalid
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	edPriv, ok := priv.(ed25519.PrivateKey)
	if !ok {
		return nil, ErrPrivateKeyInvalid
	}
	return edPriv, nil
}

func EncodePrivateKeyPEM(priv ed25519.PrivateKey) ([]byte, error) {
	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}), nil
}

func PublicKey(priv ed25519.PrivateKey) ed25519.PublicKey {
	return priv.Public().(ed25519.PublicKey)
}
