package nodefetch

import (
	"errors"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

const (
	RequestType          = "node.fetch.v1"
	ResponseType         = "node.fetch.result.v1"
	ProviderAnnounceType = "provider.announce.v1"
	Version              = 1
)

var (
	ErrNodeNotFound    = errors.New("ERR_NODE_NOT_FOUND")
	ErrNodeCIDMismatch = errors.New("ERR_NODE_CID_MISMATCH")
	ErrNodeSigInvalid  = errors.New("ERR_NODE_SIG_INVALID")
	ErrNodePolicyDeny  = errors.New("ERR_NODE_POLICY_DENY")
)

type Request struct {
	V      uint64 `cbor:"v"`
	NodeID string `cbor:"node_id"`
}

type Response struct {
	V         uint64 `cbor:"v"`
	NodeBytes []byte `cbor:"node_bytes"`
}

type ProviderAnnounce struct {
	V              uint64 `cbor:"v"`
	NodeID         string `cbor:"node_id"`
	ProviderPeerID string `cbor:"provider_peer_id"`
	TSMs           int64  `cbor:"ts_ms"`
	TTLMs          int64  `cbor:"ttl_ms"`
}

func EncodeRequest(r Request) ([]byte, error) {
	return codec.Marshal(r)
}

func DecodeRequest(data []byte) (Request, error) {
	var r Request
	err := codec.Unmarshal(data, &r)
	return r, err
}

func EncodeResponse(r Response) ([]byte, error) {
	return codec.Marshal(r)
}

func DecodeResponse(data []byte) (Response, error) {
	var r Response
	err := codec.Unmarshal(data, &r)
	return r, err
}

func EncodeProviderAnnounce(r ProviderAnnounce) ([]byte, error) {
	return codec.Marshal(r)
}

func DecodeProviderAnnounce(data []byte) (ProviderAnnounce, error) {
	var r ProviderAnnounce
	err := codec.Unmarshal(data, &r)
	return r, err
}

type Handler interface {
	Handle(NodeID string) ([]byte, error)
}

var (
	_ = ErrNodePolicyDeny
	_ = EncodeProviderAnnounce
)
