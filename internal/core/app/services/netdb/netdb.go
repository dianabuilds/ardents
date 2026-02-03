package netdb

import "github.com/dianabuilds/ardents/internal/shared/codec"

const (
	Version       = 1
	FindNodeType  = "netdb.find_node.v1"
	FindValueType = "netdb.find_value.v1"
	StoreType     = "netdb.store.v1"
	ReplyType     = "netdb.reply.v1"
)

type FindNode struct {
	V    uint64 `cbor:"v"`
	Key  []byte `cbor:"key"`
	Want string `cbor:"want"`
}

type FindValue struct {
	V   uint64 `cbor:"v"`
	Key []byte `cbor:"key"`
}

type Store struct {
	V     uint64 `cbor:"v"`
	Value []byte `cbor:"value"`
}

type Reply struct {
	V         uint64   `cbor:"v"`
	Status    string   `cbor:"status"`
	ErrorCode string   `cbor:"error_code,omitempty"`
	Nodes     []string `cbor:"nodes,omitempty"`
	Value     []byte   `cbor:"value,omitempty"`
}

var (
	_ = EncodeFindNode
	_ = EncodeFindValue
	_ = EncodeStore
)

func EncodeFindNode(r FindNode) ([]byte, error) { return codec.Marshal(r) }
func DecodeFindNode(b []byte) (FindNode, error) {
	var r FindNode
	return r, codec.Unmarshal(b, &r)
}

func EncodeFindValue(r FindValue) ([]byte, error) { return codec.Marshal(r) }
func DecodeFindValue(b []byte) (FindValue, error) {
	var r FindValue
	return r, codec.Unmarshal(b, &r)
}

func EncodeStore(r Store) ([]byte, error) { return codec.Marshal(r) }
func DecodeStore(b []byte) (Store, error) {
	var r Store
	return r, codec.Unmarshal(b, &r)
}

func EncodeReply(r Reply) ([]byte, error) { return codec.Marshal(r) }
func DecodeReply(b []byte) (Reply, error) {
	var r Reply
	return r, codec.Unmarshal(b, &r)
}
