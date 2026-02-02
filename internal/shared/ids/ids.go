package ids

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"regexp"
	"strings"

	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
)

var (
	serviceNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*\.v[1-9][0-9]*$`)
	aliasRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}[a-z0-9]$`)
)

var (
	ErrInvalidIdentityID  = errors.New("invalid identity_id")
	ErrInvalidPeerID      = errors.New("invalid peer_id")
	ErrInvalidServiceID   = errors.New("invalid service_id")
	ErrInvalidChannelID   = errors.New("invalid channel_id")
	ErrInvalidServiceName = errors.New("invalid service_name")
	ErrInvalidAlias       = errors.New("invalid alias")
)

func ValidateServiceName(name string) error {
	if !serviceNameRe.MatchString(name) {
		return ErrInvalidServiceName
	}
	return nil
}

func ValidateAlias(alias string) error {
	if !aliasRe.MatchString(alias) {
		return ErrInvalidAlias
	}
	return nil
}

func ValidateIdentityID(identityID string) error {
	if !strings.HasPrefix(identityID, "did:key:z") {
		return ErrInvalidIdentityID
	}
	_, _, err := multibase.Decode(identityID[len("did:key:"):])
	if err != nil {
		return ErrInvalidIdentityID
	}
	return nil
}

func NewIdentityID(pub ed25519.PublicKey) (string, error) {
	prefix := []byte{0xED, 0x01}
	raw := append(prefix, pub...)
	enc, err := multibase.Encode(multibase.Base58BTC, raw)
	if err != nil {
		return "", err
	}
	return "did:key:" + enc, nil
}

func IdentityPublicKey(identityID string) (ed25519.PublicKey, error) {
	if err := ValidateIdentityID(identityID); err != nil {
		return nil, err
	}
	_, data, err := multibase.Decode(identityID[len("did:key:"):])
	if err != nil {
		return nil, ErrInvalidIdentityID
	}
	if len(data) != 34 || data[0] != 0xED || data[1] != 0x01 {
		return nil, ErrInvalidIdentityID
	}
	return ed25519.PublicKey(data[2:]), nil
}

func ValidatePeerID(peerID string) error {
	if !strings.HasPrefix(peerID, "peer_") {
		return ErrInvalidPeerID
	}
	_, _, err := multibase.Decode(peerID[len("peer_"):])
	if err != nil {
		return ErrInvalidPeerID
	}
	return nil
}

func ValidateServiceID(serviceID string) error {
	if !strings.HasPrefix(serviceID, "svc_") {
		return ErrInvalidServiceID
	}
	_, _, err := multibase.Decode(serviceID[len("svc_"):])
	if err != nil {
		return ErrInvalidServiceID
	}
	return nil
}

func ValidateChannelID(channelID string) error {
	if !strings.HasPrefix(channelID, "ch_") {
		return ErrInvalidChannelID
	}
	_, _, err := multibase.Decode(channelID[len("ch_"):])
	if err != nil {
		return ErrInvalidChannelID
	}
	return nil
}

func NewPeerID(peerTransportPublicKey []byte) (string, error) {
	mh, err := multihash.Sum(peerTransportPublicKey, multihash.SHA2_256, -1)
	if err != nil {
		return "", err
	}
	enc, err := multibase.Encode(multibase.Base32, mh)
	if err != nil {
		return "", err
	}
	return "peer_" + strings.ToLower(enc), nil
}

func NewServiceID(identityID, serviceName string) (string, error) {
	if err := ValidateIdentityID(identityID); err != nil {
		return "", err
	}
	if err := ValidateServiceName(serviceName); err != nil {
		return "", err
	}
	b := bytes.Join([][]byte{[]byte(identityID), {0x00}, []byte(serviceName)}, nil)
	mh, err := multihash.Sum(b, multihash.SHA2_256, -1)
	if err != nil {
		return "", err
	}
	enc, err := multibase.Encode(multibase.Base32, mh)
	if err != nil {
		return "", err
	}
	return "svc_" + strings.ToLower(enc), nil
}

func NewChannelID(descriptorBytes []byte) (string, error) {
	mh, err := multihash.Sum(descriptorBytes, multihash.SHA2_256, -1)
	if err != nil {
		return "", err
	}
	enc, err := multibase.Encode(multibase.Base32, mh)
	if err != nil {
		return "", err
	}
	return "ch_" + strings.ToLower(enc), nil
}
