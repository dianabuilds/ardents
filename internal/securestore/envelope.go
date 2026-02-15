package securestore

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	envelopeVersion = 1
	saltSize        = 16
	filePrefix      = "AIMENC1\n"
)

var (
	ErrAuthFailed = errors.New("securestore authentication failed")
	ErrInvalid    = errors.New("securestore envelope is invalid")
	ErrLegacyData = errors.New("securestore legacy plaintext data")
)

type Envelope struct {
	Version     uint32 `json:"version"`
	KDF         string `json:"kdf"`
	KDFTime     uint32 `json:"kdf_time"`
	KDFMemoryKB uint32 `json:"kdf_memory_kb"`
	KDFThreads  uint8  `json:"kdf_threads"`
	Salt        []byte `json:"salt"`
	Nonce       []byte `json:"nonce"`
	Ciphertext  []byte `json:"ciphertext"`
}

func Encrypt(passphrase string, plaintext []byte) ([]byte, error) {
	env, err := EncryptEnvelope(passphrase, plaintext)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	return append([]byte(filePrefix), raw...), nil
}

func EncryptEnvelope(passphrase string, plaintext []byte) (*Envelope, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := deriveKey(passphrase, salt)
	defer zeroBytes(key)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	return &Envelope{
		Version:     envelopeVersion,
		KDF:         "argon2id",
		KDFTime:     2,
		KDFMemoryKB: 64 * 1024,
		KDFThreads:  1,
		Salt:        salt,
		Nonce:       nonce,
		Ciphertext:  ciphertext,
	}, nil
}

func Decrypt(passphrase string, data []byte) ([]byte, error) {
	if !strings.HasPrefix(string(data), filePrefix) {
		return nil, ErrLegacyData
	}
	data = data[len(filePrefix):]
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, ErrInvalid
	}
	return DecryptEnvelope(passphrase, &env)
}

func DecryptEnvelope(passphrase string, env *Envelope) ([]byte, error) {
	if env == nil || env.Version != envelopeVersion || env.KDF != "argon2id" {
		return nil, ErrInvalid
	}
	key := deriveKey(passphrase, env.Salt)
	defer zeroBytes(key)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, env.Nonce, env.Ciphertext, nil)
	if err != nil {
		return nil, ErrAuthFailed
	}
	return plaintext, nil
}

func deriveKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, 2, 64*1024, 1, chacha20poly1305.KeySize)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
