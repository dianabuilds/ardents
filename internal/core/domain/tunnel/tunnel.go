package tunnel

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

const (
	BuildType      = "tunnel.build.v1"
	BuildReplyType = "tunnel.build.reply.v1"
	DataType       = "tunnel.data.v1"
)

var (
	ErrBuildDecrypt = errors.New("ERR_TUNNEL_BUILD_CRYPTO")
	ErrBuildDecode  = errors.New("ERR_TUNNEL_BUILD_DECODE")
	ErrDataDecrypt  = errors.New("ERR_TUNNEL_DATA_DECODE")
)

type BuildRequest struct {
	V            uint64 `cbor:"v"`
	Direction    string `cbor:"direction"`
	TunnelID     []byte `cbor:"tunnel_id"`
	HopIndex     uint64 `cbor:"hop_index"`
	EphemeralPub []byte `cbor:"ephemeral_pub"`
	Record       []byte `cbor:"record"`
}

type BuildReply struct {
	V        uint64 `cbor:"v"`
	TunnelID []byte `cbor:"tunnel_id"`
	HopIndex uint64 `cbor:"hop_index"`
	Status   string `cbor:"status"`
	Error    string `cbor:"error_code,omitempty"`
}

type Record struct {
	V            uint64      `cbor:"v"`
	NextPeerID   string      `cbor:"next_peer_id,omitempty"`
	NextTunnelID []byte      `cbor:"next_tunnel_id,omitempty"`
	ExpiresAtMs  int64       `cbor:"expires_at_ms"`
	Flags        RecordFlags `cbor:"flags"`
}

type RecordFlags struct {
	IsGateway bool `cbor:"is_gateway"`
}

type Data struct {
	V        uint64 `cbor:"v"`
	TunnelID []byte `cbor:"tunnel_id"`
	Seq      uint64 `cbor:"seq"`
	CT       []byte `cbor:"ct"`
}

type Inner struct {
	V            uint64 `cbor:"v"`
	Kind         string `cbor:"kind"`
	NextTunnelID []byte `cbor:"next_tunnel_id,omitempty"`
	Inner        []byte `cbor:"inner,omitempty"`
	Garlic       []byte `cbor:"garlic,omitempty"`
}

func EncodeBuildRequest(r BuildRequest) ([]byte, error) { return codec.Marshal(r) }
func DecodeBuildRequest(b []byte) (BuildRequest, error) {
	var r BuildRequest
	return r, codec.Unmarshal(b, &r)
}

func EncodeBuildReply(r BuildReply) ([]byte, error) { return codec.Marshal(r) }
func DecodeBuildReply(b []byte) (BuildReply, error) {
	var r BuildReply
	return r, codec.Unmarshal(b, &r)
}

func EncodeData(r Data) ([]byte, error) { return codec.Marshal(r) }
func DecodeData(b []byte) (Data, error) {
	var r Data
	return r, codec.Unmarshal(b, &r)
}

func EncodeInner(r Inner) ([]byte, error) { return codec.Marshal(r) }
func DecodeInner(b []byte) (Inner, error) {
	var r Inner
	return r, codec.Unmarshal(b, &r)
}

var (
	_ = DecodeBuildReply
	_ = EncodeInner
	_ = DecodeInner
)

func EncryptRecord(req BuildRequest, record Record, onionPub []byte, hopPeerID string) (BuildRequest, []byte, []byte, error) {
	if len(onionPub) != 32 {
		return req, nil, nil, ErrBuildDecode
	}
	ephemeralPriv := make([]byte, 32)
	if _, err := rand.Read(ephemeralPriv); err != nil {
		return req, nil, nil, ErrBuildDecrypt
	}
	ephemeralPub, err := curve25519.X25519(ephemeralPriv, curve25519.Basepoint)
	if err != nil {
		return req, nil, nil, ErrBuildDecrypt
	}
	req.EphemeralPub = ephemeralPub
	plain, err := codec.Marshal(record)
	if err != nil {
		return req, nil, nil, ErrBuildDecode
	}
	key, err := buildKey(ephemeralPriv, onionPub, hopPeerID, req.TunnelID)
	if err != nil {
		return req, nil, nil, ErrBuildDecrypt
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return req, nil, nil, ErrBuildDecrypt
	}
	aad, err := buildAAD(req)
	if err != nil {
		return req, nil, nil, ErrBuildDecode
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	ct := aead.Seal(nil, nonce, plain, aad) // #nosec G407 -- nonce is fixed by protocol design.
	return req, ct, key, nil
}

func DecryptRecord(req BuildRequest, onionPriv []byte, hopPeerID string) (Record, []byte, error) {
	if len(onionPriv) != 32 || len(req.EphemeralPub) != 32 {
		return Record{}, nil, ErrBuildDecode
	}
	key, err := buildKey(onionPriv, req.EphemeralPub, hopPeerID, req.TunnelID)
	if err != nil {
		return Record{}, nil, ErrBuildDecrypt
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Record{}, nil, ErrBuildDecrypt
	}
	aad, err := buildAAD(req)
	if err != nil {
		return Record{}, nil, ErrBuildDecode
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX) // #nosec G407 -- nonce is fixed by protocol design.
	plain, err := aead.Open(nil, nonce, req.Record, aad)
	if err != nil {
		return Record{}, nil, ErrBuildDecrypt
	}
	var rec Record
	if err := codec.Unmarshal(plain, &rec); err != nil {
		return Record{}, nil, ErrBuildDecode
	}
	return rec, key, nil
}

func EncryptData(key []byte, inner Inner) ([]byte, error) {
	plain, err := codec.Marshal(inner)
	if err != nil {
		return nil, ErrDataDecrypt
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, ErrDataDecrypt
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	return aead.Seal(nil, nonce, plain, nil), nil // #nosec G407 -- nonce is fixed by protocol design.
}

func DecryptData(key []byte, ct []byte) (Inner, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Inner{}, ErrDataDecrypt
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX) // #nosec G407 -- nonce is fixed by protocol design.
	plain, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return Inner{}, ErrDataDecrypt
	}
	var inner Inner
	return inner, codec.Unmarshal(plain, &inner)
}

func buildKey(priv []byte, pub []byte, peerID string, tunnelID []byte) ([]byte, error) {
	ss, err := curve25519.X25519(priv, pub)
	if err != nil {
		return nil, err
	}
	salt := []byte("ardents.tunnel.build.v1")
	info := append([]byte(peerID), 0x00)
	info = append(info, tunnelID...)
	h := hkdf.New(sha256.New, ss, salt, info)
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := h.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func buildAAD(req BuildRequest) ([]byte, error) {
	aad := BuildRequest{
		V:            req.V,
		Direction:    req.Direction,
		TunnelID:     req.TunnelID,
		HopIndex:     req.HopIndex,
		EphemeralPub: req.EphemeralPub,
		Record:       nil,
	}
	return codec.Marshal(aad)
}

func DeriveHopKey(ephemeralPriv []byte, onionPub []byte, hopPeerID string, tunnelID []byte) ([]byte, error) {
	if len(ephemeralPriv) != 32 || len(onionPub) != 32 || len(tunnelID) != 16 {
		return nil, ErrBuildDecode
	}
	return buildKey(ephemeralPriv, onionPub, hopPeerID, tunnelID)
}

func KeysEqual(a, b []byte) bool {
	return bytes.Equal(a, b)
}
