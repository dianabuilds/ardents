package identity

import "time"

type Identity struct {
	ID               string
	SigningPublicKey []byte
	CreatedAt        time.Time
	LastUsedAt       time.Time
}

type DerivedKeys struct {
	SigningPrivateKey []byte // Ed25519 private key bytes (64)
	SigningPublicKey  []byte // Ed25519 public key bytes (32)
	EncryptionSeed    []byte // X25519 private seed bytes (32)
}

type EncryptedSeedEnvelope struct {
	Version     uint32 `json:"version"`
	KDF         string `json:"kdf"`
	KDFTime     uint32 `json:"kdf_time"`
	KDFMemoryKB uint32 `json:"kdf_memory_kb"`
	KDFThreads  uint8  `json:"kdf_threads"`
	Salt        []byte `json:"salt"`
	Nonce       []byte `json:"nonce"`
	Ciphertext  []byte `json:"ciphertext"`
}
