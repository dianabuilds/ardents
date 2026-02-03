package envelope

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/envelopesig"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

const Version = 1

var (
	ErrInvalidEnvelope    = errors.New("ERR_PAYLOAD_DECODE")
	ErrInvalidMsgID       = errors.New("ERR_PAYLOAD_DECODE")
	ErrInvalidFrom        = errors.New("ERR_PAYLOAD_DECODE")
	ErrInvalidTo          = errors.New("ERR_PAYLOAD_DECODE")
	ErrInvalidTTL         = errors.New("ERR_TTL_EXPIRED")
	ErrExpired            = errors.New("ERR_TTL_EXPIRED")
	ErrUnsupportedVersion = errors.New("ERR_PAYLOAD_DECODE")
)

type Envelope struct {
	V       uint64     `cbor:"v"`
	MsgID   string     `cbor:"msg_id"`
	Type    string     `cbor:"type"`
	From    From       `cbor:"from"`
	To      To         `cbor:"to"`
	TSMs    int64      `cbor:"ts_ms"`
	TTLMs   int64      `cbor:"ttl_ms"`
	Refs    []Ref      `cbor:"refs,omitempty"`
	Pow     *pow.Stamp `cbor:"pow,omitempty"`
	Payload []byte     `cbor:"payload"`
	Sig     []byte     `cbor:"sig,omitempty"`
}

type From struct {
	PeerID     string `cbor:"peer_id"`
	IdentityID string `cbor:"identity_id,omitempty"`
}

type To struct {
	PeerID    string `cbor:"peer_id,omitempty"`
	ServiceID string `cbor:"service_id,omitempty"`
	ChannelID string `cbor:"channel_id,omitempty"`
}

type Ref struct {
	Kind string `cbor:"kind"`
	ID   string `cbor:"id"`
}

func (e *Envelope) Encode() ([]byte, error) {
	return codec.Marshal(e)
}

func DecodeEnvelope(data []byte) (*Envelope, error) {
	var e Envelope
	if err := codec.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (e *Envelope) ValidateBasic(nowMs int64) error {
	if e.V != Version {
		return ErrUnsupportedVersion
	}
	if err := uuidv7.Validate(e.MsgID); err != nil {
		return ErrInvalidMsgID
	}
	if e.From.PeerID == "" || ids.ValidatePeerID(e.From.PeerID) != nil {
		return ErrInvalidFrom
	}
	if e.From.IdentityID != "" && ids.ValidateIdentityID(e.From.IdentityID) != nil {
		return ErrInvalidFrom
	}
	if !e.To.HasExactlyOne() {
		return ErrInvalidTo
	}
	if e.To.PeerID != "" && ids.ValidatePeerID(e.To.PeerID) != nil {
		return ErrInvalidTo
	}
	if e.To.ServiceID != "" && ids.ValidateServiceID(e.To.ServiceID) != nil {
		return ErrInvalidTo
	}
	if e.To.ChannelID != "" && ids.ValidateChannelID(e.To.ChannelID) != nil {
		return ErrInvalidTo
	}
	if e.TTLMs <= 0 {
		return ErrInvalidTTL
	}
	if nowMs > 0 {
		if nowMs < e.TSMs || nowMs > e.TSMs+e.TTLMs {
			return ErrExpired
		}
	}
	return nil
}

func (e *Envelope) Sign(priv ed25519.PrivateKey) error {
	if priv == nil {
		return ErrInvalidEnvelope
	}
	sig, err := envelopesig.Sign(priv, e.signingBytes)
	if err != nil {
		return err
	}
	e.Sig = sig
	return nil
}

func (e *Envelope) VerifySignature(identityID string) error {
	return envelopesig.VerifyIdentity(identityID, e.Sig, e.signingBytes, ErrInvalidEnvelope, ErrInvalidEnvelope, ErrInvalidEnvelope)
}

func (e *Envelope) signingBytes() ([]byte, error) {
	clone := *e
	clone.Sig = nil
	return codec.Marshal(&clone)
}

func (t To) HasExactlyOne() bool {
	count := 0
	if t.PeerID != "" {
		count++
	}
	if t.ServiceID != "" {
		count++
	}
	if t.ChannelID != "" {
		count++
	}
	return count == 1
}

func (e *Envelope) Age(nowMs int64) time.Duration {
	if nowMs <= 0 {
		return 0
	}
	return time.Duration(nowMs-e.TSMs) * time.Millisecond
}
