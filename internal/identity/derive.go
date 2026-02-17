package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	hkdfInfoSigning    = "aim/identity/signing/v1"
	hkdfInfoEncryption = "aim/identity/encryption/v1"
)

func DeriveKeys(seedBytes []byte) (*DerivedKeys, error) {
	signingSeed, err := hkdfExpand(seedBytes, hkdfInfoSigning, 32)
	if err != nil {
		return nil, err
	}
	encryptionSeed, err := hkdfExpand(seedBytes, hkdfInfoEncryption, 32)
	if err != nil {
		return nil, err
	}

	signingPriv := ed25519.NewKeyFromSeed(signingSeed)
	signingPub := signingPriv.Public().(ed25519.PublicKey)

	return &DerivedKeys{
		SigningPrivateKey: signingPriv,
		SigningPublicKey:  signingPub,
		EncryptionSeed:    encryptionSeed,
	}, nil
}

func hkdfExpand(seed []byte, info string, outLen int) ([]byte, error) {
	reader := hkdf.New(sha256.New, seed, nil, []byte(info))
	out := make([]byte, outLen)
	if _, err := io.ReadFull(reader, out); err != nil {
		return nil, err
	}
	return out, nil
}
