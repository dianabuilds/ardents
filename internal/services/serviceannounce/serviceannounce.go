package serviceannounce

import "github.com/dianabuilds/ardents/internal/shared/codec"

const Type = "service.announce.v1"

type Announce struct {
	V                uint64 `cbor:"v"`
	ServiceID        string `cbor:"service_id"`
	DescriptorNodeID string `cbor:"descriptor_node_id"`
	TSMs             int64  `cbor:"ts_ms"`
	TTLMs            int64  `cbor:"ttl_ms"`
}

func Encode(a Announce) ([]byte, error) {
	return codec.Marshal(a)
}

func Decode(data []byte) (Announce, error) {
	var a Announce
	err := codec.Unmarshal(data, &a)
	return a, err
}
