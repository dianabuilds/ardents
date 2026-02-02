package contentnode

import (
	"crypto/ed25519"
	"errors"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
)

var ErrInvalidNode = errors.New("invalid node")
var ErrCIDMismatch = errors.New("ERR_NODE_CID_MISMATCH")
var ErrNodeTooLarge = errors.New("ERR_NODE_TOO_LARGE")

const (
	MaxNodeBytes    = 1_048_576
	MaxLinksPerNode = 256
)

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

func EncodeWithCID(n Node) (nodeBytes []byte, nodeCID string, err error) {
	nodeBytes, err = Encode(n)
	if err != nil {
		return nil, "", err
	}
	c, err := cid.Prefix{
		Version:  1,
		Codec:    cid.DagCBOR,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(nodeBytes)
	if err != nil {
		return nil, "", err
	}
	return nodeBytes, c.String(), nil
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

func VerifyBytes(nodeBytes []byte, expectedCID string) error {
	if len(nodeBytes) > MaxNodeBytes {
		return ErrNodeTooLarge
	}
	var n Node
	if err := Decode(nodeBytes, &n); err != nil {
		return err
	}
	if n.Type == "enc.node.v1" {
		if len(n.Links) != 0 {
			return ErrInvalidNode
		}
		if n.Policy == nil {
			return ErrInvalidNode
		}
		if v, ok := n.Policy["visibility"].(string); !ok || v != "encrypted" {
			return ErrInvalidNode
		}
	}
	if len(n.Links) > MaxLinksPerNode {
		return ErrInvalidNode
	}
	if err := Verify(&n); err != nil {
		return err
	}
	c, err := cid.Prefix{
		Version:  1,
		Codec:    cid.DagCBOR,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(nodeBytes)
	if err != nil {
		return err
	}
	if expectedCID != "" && c.String() != expectedCID {
		return ErrCIDMismatch
	}
	return nil
}
