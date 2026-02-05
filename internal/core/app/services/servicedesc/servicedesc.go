package servicedesc

import (
	"crypto/ed25519"
	"errors"
	"net"
	"strings"

	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

var (
	ErrDescriptorInvalid = errors.New("ERR_SERVICE_DESCRIPTOR_INVALID")
	ErrServiceIDMismatch = errors.New("ERR_SERVICE_ID_MISMATCH")
)

var _ = []any{
	BuildDescriptorNode,
	Validate,
}

// Descriptor is the unified service descriptor model; version-specific payloads map into this.
type Descriptor struct {
	V               uint64
	OwnerIdentityID string
	ServiceName     string
	ServiceID       string
	Endpoints       []Endpoint
	Capabilities    []Capability
	Limits          map[string]uint64
	Resources       map[string]uint64
}

type descriptorBodyV1 struct {
	V            uint64            `cbor:"v"`
	ServiceName  string            `cbor:"service_name"`
	ServiceID    string            `cbor:"service_id"`
	Endpoints    []Endpoint        `cbor:"endpoints"`
	Capabilities []Capability      `cbor:"capabilities"`
	Limits       map[string]uint64 `cbor:"limits"`
}

type descriptorBodyV2 struct {
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

//nolint:unused // v1 direct-mode descriptor builder retained for compatibility
func BuildDescriptorNode(ownerID string, ownerPrivKey ed25519.PrivateKey, serviceName string, endpoints []Endpoint, caps []Capability, limits map[string]uint64) (contentnode.Node, string, error) {
	serviceID, err := buildServiceID(ownerID, serviceName, false)
	if err != nil {
		return contentnode.Node{}, "", err
	}
	body := descriptorBodyV1{
		V:            1,
		ServiceName:  serviceName,
		ServiceID:    serviceID,
		Endpoints:    endpoints,
		Capabilities: caps,
		Limits:       limits,
	}
	n := buildDescriptorNode("service.descriptor.v1", ownerID, body, timeutil.NowUnixMs())
	return encodeDescriptorNode(n, ownerPrivKey)
}

func BuildDescriptorNodeV2(ownerID string, ownerPrivKey ed25519.PrivateKey, serviceName string, caps []Capability, limits map[string]uint64, resources map[string]uint64) (contentnode.Node, string, error) {
	serviceID, err := buildServiceID(ownerID, serviceName, true)
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
	body := descriptorBodyV2{
		V:               2,
		OwnerIdentityID: ownerID,
		ServiceName:     serviceName,
		ServiceID:       serviceID,
		Capabilities:    caps,
		Limits:          limits,
		Resources:       resources,
	}
	n := buildDescriptorNode("service.descriptor.v2", ownerID, body, timeutil.NowUnixMs())
	return encodeDescriptorNode(n, ownerPrivKey)
}

// Decode maps a service.descriptor node into the unified Descriptor model.
func Decode(node contentnode.Node) (Descriptor, error) {
	switch node.Type {
	case "service.descriptor.v1":
		return decodeV1(node)
	case "service.descriptor.v2":
		return decodeV2(node)
	default:
		return Descriptor{}, ErrDescriptorInvalid
	}
}

// Validate validates the descriptor according to its version and returns the unified model.
func Validate(node contentnode.Node) (Descriptor, error) {
	body, err := Decode(node)
	if err != nil {
		return Descriptor{}, err
	}
	switch body.V {
	case 1:
		if err := validateServiceIDs(node.Owner, body.ServiceName, body.ServiceID, false); err != nil {
			return Descriptor{}, err
		}
		if err := validateEndpoints(body.Endpoints); err != nil {
			return Descriptor{}, ErrDescriptorInvalid
		}
		return body, nil
	case 2:
		if body.OwnerIdentityID == "" || body.OwnerIdentityID != node.Owner {
			return Descriptor{}, ErrDescriptorInvalid
		}
		if err := validateServiceIDs(body.OwnerIdentityID, body.ServiceName, body.ServiceID, true); err != nil {
			return Descriptor{}, err
		}
		if body.Resources == nil {
			return Descriptor{}, ErrDescriptorInvalid
		}
		if _, ok := body.Resources["cpu_cores"]; !ok {
			return Descriptor{}, ErrDescriptorInvalid
		}
		if _, ok := body.Resources["ram_mb"]; !ok {
			return Descriptor{}, ErrDescriptorInvalid
		}
		if len(body.Endpoints) > 0 {
			return Descriptor{}, ErrDescriptorInvalid
		}
		return body, nil
	default:
		return Descriptor{}, ErrDescriptorInvalid
	}
}

// ValidateV2 is a compatibility helper that enforces v2 payload rules.
func ValidateV2(node contentnode.Node) (Descriptor, error) {
	body, err := Validate(node)
	if err != nil {
		return Descriptor{}, err
	}
	if body.V != 2 {
		return Descriptor{}, ErrDescriptorInvalid
	}
	return body, nil
}

func decodeV1(node contentnode.Node) (Descriptor, error) {
	var body descriptorBodyV1
	b, err := codec.Marshal(node.Body)
	if err != nil {
		return Descriptor{}, ErrDescriptorInvalid
	}
	if err := codec.Unmarshal(b, &body); err != nil {
		return Descriptor{}, ErrDescriptorInvalid
	}
	return Descriptor{
		V:            body.V,
		ServiceName:  body.ServiceName,
		ServiceID:    body.ServiceID,
		Endpoints:    body.Endpoints,
		Capabilities: body.Capabilities,
		Limits:       body.Limits,
		// v1 uses node.Owner as the identity owner; there is no body field.
		OwnerIdentityID: node.Owner,
	}, nil
}

func decodeV2(node contentnode.Node) (Descriptor, error) {
	var body descriptorBodyV2
	b, err := codec.Marshal(node.Body)
	if err != nil {
		return Descriptor{}, ErrDescriptorInvalid
	}
	if err := codec.Unmarshal(b, &body); err != nil {
		return Descriptor{}, ErrDescriptorInvalid
	}
	return Descriptor{
		V:               body.V,
		OwnerIdentityID: body.OwnerIdentityID,
		ServiceName:     body.ServiceName,
		ServiceID:       body.ServiceID,
		Capabilities:    body.Capabilities,
		Limits:          body.Limits,
		Resources:       body.Resources,
	}, nil
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

func buildServiceID(ownerID string, serviceName string, validateOwner bool) (string, error) {
	if validateOwner {
		if err := ids.ValidateIdentityID(ownerID); err != nil {
			return "", err
		}
	}
	if err := ids.ValidateServiceName(serviceName); err != nil {
		return "", err
	}
	return ids.NewServiceID(ownerID, serviceName)
}

func validateServiceIDs(ownerID string, serviceName string, serviceID string, validateOwner bool) error {
	if validateOwner {
		if err := ids.ValidateIdentityID(ownerID); err != nil {
			return ErrDescriptorInvalid
		}
	}
	if err := ids.ValidateServiceName(serviceName); err != nil {
		return ErrDescriptorInvalid
	}
	if err := ids.ValidateServiceID(serviceID); err != nil {
		return ErrDescriptorInvalid
	}
	expectedID, err := ids.NewServiceID(ownerID, serviceName)
	if err != nil {
		return ErrDescriptorInvalid
	}
	if expectedID != serviceID {
		return ErrServiceIDMismatch
	}
	return nil
}

func buildDescriptorNode(nodeType string, ownerID string, body any, nowMs int64) contentnode.Node {
	return contentnode.Node{
		V:           1,
		Type:        nodeType,
		CreatedAtMs: nowMs,
		Owner:       ownerID,
		Links:       []contentnode.Link{},
		Body:        body,
		Policy: map[string]any{
			"v":          uint64(1),
			"visibility": "public",
		},
	}
}

func encodeDescriptorNode(n contentnode.Node, ownerPrivKey ed25519.PrivateKey) (contentnode.Node, string, error) {
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
