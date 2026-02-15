package identity

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	seedEnvelopeVersion = 1
	defaultArgonTime    = uint32(2)
	defaultArgonMemKB   = uint32(64 * 1024)
	defaultArgonThreads = uint8(1)
)

func EncryptSeed(seed []byte, password []byte) (*EncryptedSeedEnvelope, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := argon2.IDKey(password, salt, defaultArgonTime, defaultArgonMemKB, defaultArgonThreads, chacha20poly1305.KeySize)
	defer zeroBytes(key)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nil, nonce, seed, nil)
	return &EncryptedSeedEnvelope{
		Version:     seedEnvelopeVersion,
		KDF:         "argon2id",
		KDFTime:     defaultArgonTime,
		KDFMemoryKB: defaultArgonMemKB,
		KDFThreads:  defaultArgonThreads,
		Salt:        salt,
		Nonce:       nonce,
		Ciphertext:  ciphertext,
	}, nil
}

func DecryptSeed(env *EncryptedSeedEnvelope, password []byte) ([]byte, error) {
	if env.Version != seedEnvelopeVersion {
		return nil, fmt.Errorf("unsupported envelope version: %d", env.Version)
	}
	if env.KDF != "argon2id" {
		return nil, fmt.Errorf("unsupported kdf: %s", env.KDF)
	}
	key := argon2.IDKey(password, env.Salt, env.KDFTime, env.KDFMemoryKB, env.KDFThreads, chacha20poly1305.KeySize)
	defer zeroBytes(key)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, env.Nonce, env.Ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
