package envelopev2

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/envelopesig"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

const Version = 2

var (
	ErrInvalidEnvelope     = errors.New("ERR_PAYLOAD_DECODE")
	ErrInvalidMsgID        = errors.New("ERR_PAYLOAD_DECODE")
	ErrInvalidFrom         = errors.New("ERR_PAYLOAD_DECODE")
	ErrInvalidTo           = errors.New("ERR_PAYLOAD_DECODE")
	ErrInvalidTTL          = errors.New("ERR_ENV2_TTL_EXPIRED")
	ErrExpired             = errors.New("ERR_ENV2_TTL_EXPIRED")
	ErrSigRequired         = errors.New("ERR_ENV2_SIG_REQUIRED")
	ErrSigInvalid          = errors.New("ERR_ENV2_SIG_INVALID")
	ErrFromServiceMismatch = errors.New("ERR_FROM_SERVICE_MISMATCH")
	ErrUnsupportedVersion  = errors.New("ERR_PAYLOAD_DECODE")
)

type Envelope struct {
	V       uint64 `cbor:"v"`
	MsgID   string `cbor:"msg_id"`
	Type    string `cbor:"type"`
	From    From   `cbor:"from"`
	To      To     `cbor:"to"`
	TSMs    int64  `cbor:"ts_ms"`
	TTLMs   int64  `cbor:"ttl_ms"`
	ReplyTo *Reply `cbor:"reply_to,omitempty"`
	Refs    []Ref  `cbor:"refs,omitempty"`
	Payload []byte `cbor:"payload"`
	Sig     []byte `cbor:"sig,omitempty"`
}

type From struct {
	IdentityID string `cbor:"identity_id,omitempty"`
	ServiceID  string `cbor:"service_id,omitempty"`
}

type To struct {
	ServiceID string `cbor:"service_id"`
}

type Reply struct {
	ServiceID string `cbor:"service_id"`
}

type Ref struct {
	Kind string `cbor:"kind"`
	ID   string `cbor:"id"`
}

func (e *Envelope) Encode() ([]byte, error) { return codec.Marshal(e) }

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
	if e.Type == "" {
		return ErrInvalidEnvelope
	}
	if e.From.IdentityID != "" && ids.ValidateIdentityID(e.From.IdentityID) != nil {
		return ErrInvalidFrom
	}
	if e.From.ServiceID != "" {
		if e.From.IdentityID == "" {
			return ErrFromServiceMismatch
		}
		if ids.ValidateServiceID(e.From.ServiceID) != nil {
			return ErrInvalidFrom
		}
	}
	if e.To.ServiceID == "" || ids.ValidateServiceID(e.To.ServiceID) != nil {
		return ErrInvalidTo
	}
	if e.ReplyTo != nil && e.ReplyTo.ServiceID != "" {
		if ids.ValidateServiceID(e.ReplyTo.ServiceID) != nil {
			return ErrInvalidTo
		}
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
	return envelopesig.VerifyIdentity(identityID, e.Sig, e.signingBytes, ErrInvalidEnvelope, ErrSigRequired, ErrSigInvalid)
}

func (e *Envelope) signingBytes() ([]byte, error) {
	clone := *e
	clone.Sig = nil
	return codec.Marshal(&clone)
}

func (e *Envelope) Age(nowMs int64) time.Duration {
	if nowMs <= 0 {
		return 0
	}
	return time.Duration(nowMs-e.TSMs) * time.Millisecond
}
