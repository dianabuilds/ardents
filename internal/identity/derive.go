package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/mr-tron/base58/base58"
	"golang.org/x/crypto/blake2b"
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

func hkdfExpand(seed []byte, info string, outLen int) ([]byte, error) {
	reader := hkdf.New(sha256.New, seed, nil, []byte(info))
	out := make([]byte, outLen)
	if _, err := io.ReadFull(reader, out); err != nil {
		return nil, err
	}
	return out, nil
}
