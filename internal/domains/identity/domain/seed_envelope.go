package domain

import (
	"fmt"

	"aim-chat/go-backend/internal/securestore"
)

func EncryptSeed(seed []byte, password []byte) (*EncryptedSeedEnvelope, error) {
	env, err := securestore.EncryptEnvelope(string(password), seed)
	if err != nil {
		return nil, err
	}
	return &EncryptedSeedEnvelope{
		Version:     env.Version,
		KDF:         env.KDF,
		KDFTime:     env.KDFTime,
		KDFMemoryKB: env.KDFMemoryKB,
		KDFThreads:  env.KDFThreads,
		Salt:        env.Salt,
		Nonce:       env.Nonce,
		Ciphertext:  env.Ciphertext,
	}, nil
}

func DecryptSeed(env *EncryptedSeedEnvelope, password []byte) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("seed envelope is nil")
	}
	return securestore.DecryptEnvelope(string(password), &securestore.Envelope{
		Version:     env.Version,
		KDF:         env.KDF,
		KDFTime:     env.KDFTime,
		KDFMemoryKB: env.KDFMemoryKB,
		KDFThreads:  env.KDFThreads,
		Salt:        env.Salt,
		Nonce:       env.Nonce,
		Ciphertext:  env.Ciphertext,
	})
}
