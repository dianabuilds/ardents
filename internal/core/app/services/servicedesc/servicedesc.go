package servicedesc

import (
	"crypto/ed25519"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
)

var (
	ErrDescriptorInvalid = errors.New("ERR_SERVICE_DESCRIPTOR_INVALID")
	ErrServiceIDMismatch = errors.New("ERR_SERVICE_ID_MISMATCH")
)

type DescriptorBody struct {
	V            uint64            `cbor:"v"`
	ServiceName  string            `cbor:"service_name"`
	ServiceID    string            `cbor:"service_id"`
	Endpoints    []Endpoint        `cbor:"endpoints"`
	Capabilities []Capability      `cbor:"capabilities"`
	Limits       map[string]uint64 `cbor:"limits"`
}

type DescriptorBodyV2 struct {
	V               uint64            `cbor:"v"`
	OwnerIdentityID string            `cbor:"owner_identity_id"`
	ServiceName     string            `cbor:"service_name"`
	ServiceID       string            `cbor:"service_id"`
	Capabilities    []Capability      `cbor:"capabilities"`
	Limits          map[string]uint64 `cbor:"limits"`
	Resources       map[string]uint64 `cbor:"resources"`
}

type Endpoint struct {
	PeerID   string   `cbor:"peer_id"`
	Addrs    []string `cbor:"addrs"`
	Priority uint64   `cbor:"priority"`
}

type Capability struct {
	V       uint64   `cbor:"v"`
	JobType string   `cbor:"job_type"`
	Modes   []string `cbor:"modes"`
}

func BuildDescriptorNode(ownerID string, ownerPrivKey ed25519.PrivateKey, serviceName string, endpoints []Endpoint, caps []Capability, limits map[string]uint64) (contentnode.Node, string, error) {
	if err := ids.ValidateServiceName(serviceName); err != nil {
		return contentnode.Node{}, "", err
	}
	serviceID, err := ids.NewServiceID(ownerID, serviceName)
	if err != nil {
		return contentnode.Node{}, "", err
	}
	body := DescriptorBody{
		V:            1,
		ServiceName:  serviceName,
		ServiceID:    serviceID,
		Endpoints:    endpoints,
		Capabilities: caps,
		Limits:       limits,
	}
	n := contentnode.Node{
		V:           1,
		Type:        "service.descriptor.v1",
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Owner:       ownerID,
		Links:       []contentnode.Link{},
		Body:        body,
		Policy: map[string]any{
			"v":          uint64(1),
			"visibility": "public",
		},
	}
	if err := contentnode.Sign(&n, ownerPrivKey); err != nil {
		return contentnode.Node{}, "", err
	}
	nodeBytes, nodeID, err := contentnode.EncodeWithCID(n)
	if err != nil {
		return contentnode.Node{}, "", err
	}
	if err := contentnode.VerifyBytes(nodeBytes, nodeID); err != nil {
		return contentnode.Node{}, "", err
	}
	return n, nodeID, nil
}

func BuildDescriptorNodeV2(ownerID string, ownerPrivKey ed25519.PrivateKey, serviceName string, caps []Capability, limits map[string]uint64, resources map[string]uint64) (contentnode.Node, string, error) {
	if err := ids.ValidateIdentityID(ownerID); err != nil {
		return contentnode.Node{}, "", err
	}
	if err := ids.ValidateServiceName(serviceName); err != nil {
		return contentnode.Node{}, "", err
	}
	serviceID, err := ids.NewServiceID(ownerID, serviceName)
	if err != nil {
		return contentnode.Node{}, "", err
	}
	if limits == nil {
		limits = map[string]uint64{}
	}
	if resources == nil {
		resources = map[string]uint64{}
	}
	if _, ok := resources["cpu_cores"]; !ok {
		resources["cpu_cores"] = 0
	}
	if _, ok := resources["ram_mb"]; !ok {
		resources["ram_mb"] = 0
	}
	body := DescriptorBodyV2{
		V:               2,
		OwnerIdentityID: ownerID,
		ServiceName:     serviceName,
		ServiceID:       serviceID,
		Capabilities:    caps,
		Limits:          limits,
		Resources:       resources,
	}
	n := contentnode.Node{
		V:           1,
		Type:        "service.descriptor.v2",
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Owner:       ownerID,
		Links:       []contentnode.Link{},
		Body:        body,
		Policy: map[string]any{
			"v":          uint64(1),
			"visibility": "public",
		},
	}
	if err := contentnode.Sign(&n, ownerPrivKey); err != nil {
		return contentnode.Node{}, "", err
	}
	nodeBytes, nodeID, err := contentnode.EncodeWithCID(n)
	if err != nil {
		return contentnode.Node{}, "", err
	}
	if err := contentnode.VerifyBytes(nodeBytes, nodeID); err != nil {
		return contentnode.Node{}, "", err
	}
	return n, nodeID, nil
}

func DecodeBody(node contentnode.Node) (DescriptorBody, error) {
	if node.Type != "service.descriptor.v1" {
		return DescriptorBody{}, ErrDescriptorInvalid
	}
	var body DescriptorBody
	b, err := codec.Marshal(node.Body)
	if err != nil {
		return DescriptorBody{}, ErrDescriptorInvalid
	}
	if err := codec.Unmarshal(b, &body); err != nil {
		return DescriptorBody{}, ErrDescriptorInvalid
	}
	return body, nil
}

func DecodeBodyV2(node contentnode.Node) (DescriptorBodyV2, error) {
	if node.Type != "service.descriptor.v2" {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	var body DescriptorBodyV2
	b, err := codec.Marshal(node.Body)
	if err != nil {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	if err := codec.Unmarshal(b, &body); err != nil {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	return body, nil
}

func Validate(node contentnode.Node) (DescriptorBody, error) {
	body, err := DecodeBody(node)
	if err != nil {
		return DescriptorBody{}, err
	}
	if body.V != 1 {
		return DescriptorBody{}, ErrDescriptorInvalid
	}
	if err := ids.ValidateServiceName(body.ServiceName); err != nil {
		return DescriptorBody{}, ErrDescriptorInvalid
	}
	if err := ids.ValidateServiceID(body.ServiceID); err != nil {
		return DescriptorBody{}, ErrDescriptorInvalid
	}
	expectedID, err := ids.NewServiceID(node.Owner, body.ServiceName)
	if err != nil {
		return DescriptorBody{}, ErrDescriptorInvalid
	}
	if expectedID != body.ServiceID {
		return DescriptorBody{}, ErrServiceIDMismatch
	}
	if err := validateEndpoints(body.Endpoints); err != nil {
		return DescriptorBody{}, ErrDescriptorInvalid
	}
	return body, nil
}

func ValidateV2(node contentnode.Node) (DescriptorBodyV2, error) {
	body, err := DecodeBodyV2(node)
	if err != nil {
		return DescriptorBodyV2{}, err
	}
	if body.V != 2 {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	if body.OwnerIdentityID == "" || body.OwnerIdentityID != node.Owner {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	if err := ids.ValidateServiceName(body.ServiceName); err != nil {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	if err := ids.ValidateServiceID(body.ServiceID); err != nil {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	expectedID, err := ids.NewServiceID(body.OwnerIdentityID, body.ServiceName)
	if err != nil {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	if expectedID != body.ServiceID {
		return DescriptorBodyV2{}, ErrServiceIDMismatch
	}
	if body.Resources == nil {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	if _, ok := body.Resources["cpu_cores"]; !ok {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	if _, ok := body.Resources["ram_mb"]; !ok {
		return DescriptorBodyV2{}, ErrDescriptorInvalid
	}
	return body, nil
}

func validateEndpoints(endpoints []Endpoint) error {
	for _, ep := range endpoints {
		if ep.PeerID != "" {
			if err := ids.ValidatePeerID(ep.PeerID); err != nil {
				return err
			}
		}
		for _, addr := range ep.Addrs {
			if err := validateAddr(addr); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAddr(addr string) error {
	if !strings.HasPrefix(addr, "quic://") {
		return ErrDescriptorInvalid
	}
	raw := strings.TrimPrefix(addr, "quic://")
	if _, _, err := net.SplitHostPort(raw); err != nil {
		return ErrDescriptorInvalid
	}
	return nil
}
