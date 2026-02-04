package quic

import (
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

const HelloVersion = 1

var (
	ErrHandshakeTimeSkew  = errors.New("ERR_HANDSHAKE_TIME_SKEW")
	ErrUnsupportedVersion = errors.New("ERR_UNSUPPORTED_VERSION")
	ErrAddrInvalid        = errors.New("ERR_ADDR_INVALID")
)

type Hello struct {
	V                  uint64 `cbor:"v"`
	PeerID             string `cbor:"peer_id"`
	TSMs               int64  `cbor:"ts_ms"`
	Nonce              []byte `cbor:"nonce"`
	PowDifficulty      uint64 `cbor:"pow_difficulty"`
	MaxMsgBytes        uint64 `cbor:"max_msg_bytes"`
	CapabilitiesDigest []byte `cbor:"capabilities_digest"`
	// Optional handshake hint for faster discovery/bootstrap.
	// Must be validated by receiver (see SPEC-110/SPEC-460) and treated as short-lived.
	RouterInfo []byte `cbor:"router_info,omitempty"`
	// Optional list of additional router.info hints (each is a standalone signed record).
	RouterInfos [][]byte `cbor:"router_infos,omitempty"`
}

func EncodeHello(h Hello) ([]byte, error) {
	return codec.Marshal(h)
}

func DecodeHello(data []byte) (Hello, error) {
	var h Hello
	err := codec.Unmarshal(data, &h)
	return h, err
}

func ValidateHello(localNowMs int64, hello Hello) error {
	if hello.V != HelloVersion {
		return ErrUnsupportedVersion
	}
	if hello.PeerID == "" {
		return ErrPeerIDMismatch
	}
	if hello.TSMs <= 0 {
		return ErrHandshakeTimeSkew
	}
	if localNowMs > 0 {
		diff := time.Duration(localNowMs-hello.TSMs) * time.Millisecond
		if diff < 0 {
			diff = -diff
		}
		if diff > 5*time.Minute {
			return ErrHandshakeTimeSkew
		}
	}
	return nil
}
