package dirquery

import (
	"errors"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

const (
	JobType  = "dir.query.v1"
	NodeType = "dir.query.result.v1"
	Version  = 1
)

var ErrInputInvalid = errors.New("ERR_INPUT_INVALID")

type Input struct {
	V     uint64 `cbor:"v"`
	Query Query  `cbor:"query"`
	Limit uint64 `cbor:"limit"`
}

type Query struct {
	ServiceNamePrefix string     `cbor:"service_name_prefix,omitempty"`
	Requires          []string   `cbor:"requires,omitempty"`
	MinResources      *Resources `cbor:"min_resources,omitempty"`
}

type Resources struct {
	CpuCores uint64 `cbor:"cpu_cores,omitempty"`
	RamMB    uint64 `cbor:"ram_mb,omitempty"`
}

type ResultBody struct {
	V           uint64       `cbor:"v"`
	QueryHash   []byte       `cbor:"query_hash"`
	IssuedAtMs  int64        `cbor:"issued_at_ms"`
	ExpiresAtMs int64        `cbor:"expires_at_ms"`
	Results     []ResultItem `cbor:"results"`
}

type ResultItem struct {
	ServiceID          string `cbor:"service_id"`
	OwnerIdentityID    string `cbor:"owner_identity_id"`
	ServiceName        string `cbor:"service_name"`
	CapabilitiesDigest []byte `cbor:"capabilities_digest"`
	Score              int64  `cbor:"score"`
}

func DecodeInput(input map[string]any) (Input, error) {
	if input == nil {
		return Input{}, ErrInputInvalid
	}
	b, err := codec.Marshal(input)
	if err != nil {
		return Input{}, ErrInputInvalid
	}
	var out Input
	if err := codec.Unmarshal(b, &out); err != nil {
		return Input{}, ErrInputInvalid
	}
	if out.V != 1 {
		return Input{}, ErrInputInvalid
	}
	if out.Limit > 50 {
		return Input{}, ErrInputInvalid
	}
	for _, req := range out.Query.Requires {
		if req == "" {
			return Input{}, ErrInputInvalid
		}
	}
	return out, nil
}
