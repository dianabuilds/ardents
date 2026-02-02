package contentnode

import (
	"crypto/ed25519"
	"errors"

	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
)

var ErrInvalidNode = errors.New("invalid node")

type Node struct {
	V           uint64         `cbor:"v"`
	Type        string         `cbor:"type"`
	CreatedAtMs int64          `cbor:"created_at_ms"`
	Owner       string         `cbor:"owner"`
	Links       []Link         `cbor:"links"`
	Body        any            `cbor:"body"`
	Policy      map[string]any `cbor:"policy"`
	Sig         []byte         `cbor:"sig"`
}

type Link struct {
	Rel    string `cbor:"rel"`
	NodeID string `cbor:"node_id"`
}

func Encode(n Node) ([]byte, error) {
	return codec.Marshal(n)
}

func Decode(data []byte, n *Node) error {
	return codec.Unmarshal(data, n)
}

func Sign(n *Node, priv ed25519.PrivateKey) error {
	if n == nil || priv == nil {
		return ErrInvalidNode
	}
	clone := *n
	clone.Sig = nil
	b, err := codec.Marshal(clone)
	if err != nil {
		return err
	}
	n.Sig = ed25519.Sign(priv, b)
	return nil
}

func Verify(n *Node) error {
	if n == nil || n.Owner == "" || len(n.Sig) == 0 {
		return ErrInvalidNode
	}
	pub, err := ids.IdentityPublicKey(n.Owner)
	if err != nil {
		return err
	}
	clone := *n
	clone.Sig = nil
	b, err := codec.Marshal(clone)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, b, n.Sig) {
		return ErrInvalidNode
	}
	return nil
}
