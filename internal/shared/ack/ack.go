package ack

import "github.com/dianabuilds/ardents/internal/shared/codec"

const Version = 1

type Payload struct {
	V           uint64 `cbor:"v"`
	AckForMsgID string `cbor:"ack_for_msg_id"`
	Status      string `cbor:"status"`
	ErrorCode   string `cbor:"error_code,omitempty"`
}

func Encode(p Payload) ([]byte, error) {
	return codec.Marshal(p)
}

func Decode(data []byte) (Payload, error) {
	var p Payload
	err := codec.Unmarshal(data, &p)
	return p, err
}
