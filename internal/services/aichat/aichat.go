package aichat

import (
	"errors"
	"strings"

	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
)

var (
	ErrPolicyInvalid = errors.New("ERR_AI_POLICY_INVALID")
	ErrInputTooLarge = errors.New("ERR_AI_INPUT_TOO_LARGE")
	ErrInputInvalid  = errors.New("ERR_AI_INPUT_INVALID")
)

type Message struct {
	Role    string `cbor:"role"`
	Content string `cbor:"content"`
}

type Policy struct {
	Visibility string   `cbor:"visibility"`
	Recipients []string `cbor:"recipients,omitempty"`
}

type Input struct {
	V        uint64         `cbor:"v"`
	Messages []Message      `cbor:"messages"`
	Params   map[string]any `cbor:"params,omitempty"`
	Policy   Policy         `cbor:"policy"`
}

func DecodeInput(input map[string]any, maxBytes uint64) (Input, error) {
	if input == nil {
		return Input{}, ErrInputInvalid
	}
	raw, err := codec.Marshal(input)
	if err != nil {
		return Input{}, ErrInputInvalid
	}
	if maxBytes > 0 && uint64(len(raw)) > maxBytes {
		return Input{}, ErrInputTooLarge
	}
	var out Input
	if err := codec.Unmarshal(raw, &out); err != nil {
		return Input{}, ErrInputInvalid
	}
	if out.V != 1 {
		return Input{}, ErrInputInvalid
	}
	if len(out.Messages) == 0 {
		return Input{}, ErrInputInvalid
	}
	for _, msg := range out.Messages {
		role := strings.ToLower(msg.Role)
		if role != "system" && role != "user" && role != "assistant" {
			return Input{}, ErrInputInvalid
		}
	}
	if out.Policy.Visibility != "public" && out.Policy.Visibility != "encrypted" {
		return Input{}, ErrPolicyInvalid
	}
	if out.Policy.Visibility == "encrypted" {
		if len(out.Policy.Recipients) == 0 {
			return Input{}, ErrPolicyInvalid
		}
		for _, rid := range out.Policy.Recipients {
			if err := ids.ValidateIdentityID(rid); err != nil {
				return Input{}, ErrPolicyInvalid
			}
		}
	}
	return out, nil
}
