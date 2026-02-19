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
	argonTime       = uint32(2)
	argonMemoryKB   = uint32(64 * 1024)
	argonThreads    = uint8(1)
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
		KDFTime:     argonTime,
		KDFMemoryKB: argonMemoryKB,
		KDFThreads:  argonThreads,
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
	if !isValidEnvelope(env) {
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
	return argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemoryKB, argonThreads, chacha20poly1305.KeySize)
}

func isValidEnvelope(env *Envelope) bool {
	if env == nil {
		return false
	}
	if env.Version != envelopeVersion || env.KDF != "argon2id" {
		return false
	}
	if env.KDFTime != argonTime || env.KDFMemoryKB != argonMemoryKB || env.KDFThreads != argonThreads {
		return false
	}
	if len(env.Salt) != saltSize || len(env.Nonce) != chacha20poly1305.NonceSizeX || len(env.Ciphertext) == 0 {
		return false
	}
	return true
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
