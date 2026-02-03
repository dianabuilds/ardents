package relay

import (
	"crypto/ed25519"
	"errors"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

const Version = 1

var ErrInvalidPacket = errors.New("ERR_RELAY_PACKET_INVALID")

type Packet struct {
	V          uint64 `cbor:"v"`
	NextPeerID string `cbor:"next_peer_id"`
	TTLMs      int64  `cbor:"ttl_ms"`
	Inner      []byte `cbor:"inner"`
}

func Encode(p Packet) ([]byte, error) {
	return codec.Marshal(p)
}

func Decode(data []byte) (Packet, error) {
	var p Packet
	if err := codec.Unmarshal(data, &p); err != nil {
		return Packet{}, err
	}
	return p, nil
}

func Validate(p Packet) error {
	if p.V != Version || p.NextPeerID == "" || p.TTLMs <= 0 || len(p.Inner) == 0 {
		return ErrInvalidPacket
	}
	return nil
}

func Build(nextPeerID string, ttlMs int64, inner []byte, recipientPub ed25519.PublicKey) (Packet, error) {
	if nextPeerID == "" || ttlMs <= 0 || len(inner) == 0 {
		return Packet{}, ErrInvalidPacket
	}
	sealed, err := SealInner(recipientPub, inner)
	if err != nil {
		return Packet{}, err
	}
	return Packet{
		V:          Version,
		NextPeerID: nextPeerID,
		TTLMs:      ttlMs,
		Inner:      sealed,
	}, nil
}
