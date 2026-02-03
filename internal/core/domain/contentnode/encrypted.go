package contentnode

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"sort"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/cryptobox"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"golang.org/x/crypto/chacha20poly1305"
)

var (
	ErrEncUnsupported   = errors.New("ERR_ENC_UNSUPPORTED")
	ErrEncNoRecipient   = errors.New("ERR_ENC_NO_RECIPIENT")
	ErrEncDecryptFailed = errors.New("ERR_ENC_DECRYPT_FAILED")
)

type EncryptedBody struct {
	V          uint64      `cbor:"v"`
	Alg        string      `cbor:"alg"`
	Recipients []Recipient `cbor:"recipients"`
	Ciphertext []byte      `cbor:"ciphertext"`
	Nonce      []byte      `cbor:"nonce"`
}

type Recipient struct {
	IdentityID string `cbor:"identity_id"`
	SealedKey  []byte `cbor:"sealed_key"`
}

type PrivateNodePayload struct {
	V     uint64 `cbor:"v"`
	Type  string `cbor:"type"`
	Links []Link `cbor:"links"`
	Body  any    `cbor:"body"`
}

func EncryptNode(ownerID string, ownerPriv ed25519.PrivateKey, nodeType string, links []Link, body any, recipients []string) (Node, error) {
	if len(recipients) == 0 {
		return Node{}, ErrEncNoRecipient
	}
	payload := PrivateNodePayload{
		V:     1,
		Type:  nodeType,
		Links: links,
		Body:  body,
	}
	plain, err := codec.Marshal(payload)
	if err != nil {
		return Node{}, err
	}
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := rand.Read(key); err != nil {
		return Node{}, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return Node{}, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Node{}, err
	}
	ciphertext := aead.Seal(nil, nonce, plain, nil)
	rcpts := make([]Recipient, 0, len(recipients))
	for _, id := range recipients {
		pub, err := ids.IdentityPublicKey(id)
		if err != nil {
			return Node{}, err
		}
		sealed, err := cryptobox.SealAnonymous(rand.Reader, pub, key)
		if err != nil {
			return Node{}, err
		}
		rcpts = append(rcpts, Recipient{
			IdentityID: id,
			SealedKey:  sealed,
		})
	}
	sort.Slice(rcpts, func(i, j int) bool {
		return rcpts[i].IdentityID < rcpts[j].IdentityID
	})
	encBody := EncryptedBody{
		V:          1,
		Alg:        "xchacha20poly1305",
		Recipients: rcpts,
		Ciphertext: ciphertext,
		Nonce:      nonce,
	}
	n := Node{
		V:           1,
		Type:        "enc.node.v1",
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Owner:       ownerID,
		Links:       []Link{},
		Body:        encBody,
		Policy: map[string]any{
			"v":          uint64(1),
			"visibility": "encrypted",
		},
	}
	if err := Sign(&n, ownerPriv); err != nil {
		return Node{}, err
	}
	return n, nil
}

func DecryptNode(n Node, recipientID string, recipientPriv ed25519.PrivateKey) (PrivateNodePayload, error) {
	if n.Type != "enc.node.v1" {
		return PrivateNodePayload{}, ErrEncUnsupported
	}
	body, ok := n.Body.(EncryptedBody)
	if !ok {
		var enc EncryptedBody
		b, err := codec.Marshal(n.Body)
		if err != nil {
			return PrivateNodePayload{}, ErrEncDecryptFailed
		}
		if err := codec.Unmarshal(b, &enc); err != nil {
			return PrivateNodePayload{}, ErrEncDecryptFailed
		}
		body = enc
	}
	if body.Alg != "xchacha20poly1305" {
		return PrivateNodePayload{}, ErrEncUnsupported
	}
	var sealed []byte
	for _, r := range body.Recipients {
		if r.IdentityID == recipientID {
			sealed = r.SealedKey
			break
		}
	}
	if len(sealed) == 0 {
		return PrivateNodePayload{}, ErrEncNoRecipient
	}
	pub, err := ids.IdentityPublicKey(recipientID)
	if err != nil {
		return PrivateNodePayload{}, ErrEncDecryptFailed
	}
	key, err := cryptobox.OpenAnonymous(pub, recipientPriv, sealed)
	if err != nil {
		return PrivateNodePayload{}, ErrEncDecryptFailed
	}
	if len(body.Nonce) != chacha20poly1305.NonceSizeX {
		return PrivateNodePayload{}, ErrEncDecryptFailed
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return PrivateNodePayload{}, ErrEncDecryptFailed
	}
	plain, err := aead.Open(nil, body.Nonce, body.Ciphertext, nil)
	if err != nil {
		return PrivateNodePayload{}, ErrEncDecryptFailed
	}
	var payload PrivateNodePayload
	if err := codec.Unmarshal(plain, &payload); err != nil {
		return PrivateNodePayload{}, ErrEncDecryptFailed
	}
	return payload, nil
}
