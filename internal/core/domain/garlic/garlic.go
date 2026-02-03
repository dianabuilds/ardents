package garlic

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

const Version = 1

var (
	ErrGarlicDecode  = errors.New("ERR_GARLIC_DECODE")
	ErrGarlicDecrypt = errors.New("ERR_GARLIC_DECRYPT")
	ErrGarlicExpired = errors.New("ERR_GARLIC_EXPIRED")
	ErrGarlicPayload = errors.New("ERR_GARLIC_PAYLOAD")
)

type Message struct {
	V            uint64 `cbor:"v"`
	ToServiceID  string `cbor:"to_service_id"`
	EphemeralPub []byte `cbor:"ephemeral_pub"`
	CT           []byte `cbor:"ct"`
}

type Inner struct {
	V           uint64  `cbor:"v"`
	ExpiresAtMs int64   `cbor:"expires_at_ms"`
	Cloves      []Clove `cbor:"cloves"`
}

type Clove struct {
	Kind     string `cbor:"kind"`
	Envelope []byte `cbor:"envelope"`
}

func Encode(msg Message) ([]byte, error) { return codec.Marshal(msg) }

func Decode(data []byte) (Message, error) {
	var m Message
	if err := codec.Unmarshal(data, &m); err != nil {
		return Message{}, err
	}
	return m, nil
}

func Encrypt(toServiceID string, encPub []byte, inner Inner) (Message, error) {
	if toServiceID == "" || len(encPub) != 32 {
		return Message{}, ErrGarlicPayload
	}
	ephemeralPriv := make([]byte, 32)
	if _, err := rand.Read(ephemeralPriv); err != nil {
		return Message{}, ErrGarlicDecrypt
	}
	ephemeralPub, err := curve25519.X25519(ephemeralPriv, curve25519.Basepoint)
	if err != nil {
		return Message{}, ErrGarlicDecrypt
	}
	header := Message{
		V:            Version,
		ToServiceID:  toServiceID,
		EphemeralPub: ephemeralPub,
	}
	aad, err := headerAAD(header)
	if err != nil {
		return Message{}, ErrGarlicDecode
	}
	plain, err := codec.Marshal(inner)
	if err != nil {
		return Message{}, ErrGarlicDecode
	}
	key, err := garlicKey(ephemeralPriv, encPub, toServiceID)
	if err != nil {
		return Message{}, ErrGarlicDecrypt
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Message{}, ErrGarlicDecrypt
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	ct := aead.Seal(nil, nonce, plain, aad) // #nosec G407 -- nonce is fixed by protocol design.
	header.CT = ct
	return header, nil
}

func Decrypt(msg Message, encPriv []byte) (Inner, error) {
	if msg.V != Version || msg.ToServiceID == "" || len(msg.EphemeralPub) != 32 {
		return Inner{}, ErrGarlicDecode
	}
	if len(encPriv) != 32 {
		return Inner{}, ErrGarlicDecrypt
	}
	aad, err := headerAAD(msg)
	if err != nil {
		return Inner{}, ErrGarlicDecode
	}
	key, err := garlicKey(encPriv, msg.EphemeralPub, msg.ToServiceID)
	if err != nil {
		return Inner{}, ErrGarlicDecrypt
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Inner{}, ErrGarlicDecrypt
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX) // #nosec G407 -- nonce is fixed by protocol design.
	plain, err := aead.Open(nil, nonce, msg.CT, aad)
	if err != nil {
		return Inner{}, ErrGarlicDecrypt
	}
	var inner Inner
	if err := codec.Unmarshal(plain, &inner); err != nil {
		return Inner{}, ErrGarlicDecode
	}
	return inner, nil
}

func headerAAD(msg Message) ([]byte, error) {
	h := Message{
		V:            msg.V,
		ToServiceID:  msg.ToServiceID,
		EphemeralPub: msg.EphemeralPub,
		CT:           nil,
	}
	return codec.Marshal(h)
}

func garlicKey(priv []byte, pub []byte, toServiceID string) ([]byte, error) {
	ss, err := curve25519.X25519(priv, pub)
	if err != nil {
		return nil, err
	}
	salt := []byte("ardents.garlic.v1")
	info := []byte(toServiceID)
	h := hkdf.New(sha256.New, ss, salt, info)
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := h.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}
