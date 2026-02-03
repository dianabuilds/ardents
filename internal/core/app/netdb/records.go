package netdb

import (
	"crypto/ed25519"

	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/routerinfo"
	"github.com/ipfs/go-cid"
)

func (db *DB) validateRouterInfo(r RouterInfo, nowMs int64) error {
	if r.V != 1 {
		return ErrBadRecord
	}
	if err := ids.ValidatePeerID(r.PeerID); err != nil {
		return ErrBadRecord
	}
	if len(r.TransportPub) != ed25519.PublicKeySize || len(r.OnionPub) != 32 {
		return ErrBadRecord
	}
	if len(r.Addrs) == 0 || len(r.Addrs) > MaxAddrsPerRouter {
		return ErrBadRecord
	}
	if !r.Caps.Relay || !r.Caps.NetDB {
		return ErrBadRecord
	}
	if r.ExpiresAtMs <= r.PublishedAtMs {
		return ErrBadRecord
	}
	if r.ExpiresAtMs <= nowMs {
		return ErrExpired
	}
	if r.ExpiresAtMs-r.PublishedAtMs > db.maxTTLms {
		return ErrBadRecord
	}
	peerID, err := ids.NewPeerID(r.TransportPub)
	if err != nil || peerID != r.PeerID {
		return ErrBadRecord
	}
	unsigned, err := unsignedRouterBytes(r)
	if err != nil {
		return ErrBadRecord
	}
	if !ed25519.Verify(ed25519.PublicKey(r.TransportPub), unsigned, r.Sig) {
		return ErrSigInvalid
	}
	return nil
}

func (db *DB) validateLeaseSet(s LeaseSet, nowMs int64) error {
	if s.V != 1 {
		return ErrBadRecord
	}
	if err := ids.ValidateServiceName(s.ServiceName); err != nil {
		return ErrBadRecord
	}
	if err := ids.ValidateServiceID(s.ServiceID); err != nil {
		return ErrBadRecord
	}
	svcID, err := ids.NewServiceID(s.OwnerIdentityID, s.ServiceName)
	if err != nil || svcID != s.ServiceID {
		return ErrBadRecord
	}
	if len(s.EncPub) != 32 {
		return ErrBadRecord
	}
	if len(s.Leases) == 0 || len(s.Leases) > MaxLeasesPerLeaseSet {
		return ErrBadRecord
	}
	minLease := int64(0)
	for i, l := range s.Leases {
		if err := ids.ValidatePeerID(l.GatewayPeerID); err != nil {
			return ErrBadRecord
		}
		if len(l.TunnelID) != 16 {
			return ErrBadRecord
		}
		if l.ExpiresAtMs <= nowMs {
			return ErrExpired
		}
		if i == 0 || l.ExpiresAtMs < minLease {
			minLease = l.ExpiresAtMs
		}
	}
	if s.ExpiresAtMs > minLease {
		return ErrBadRecord
	}
	if s.ExpiresAtMs <= s.PublishedAtMs {
		return ErrBadRecord
	}
	if s.ExpiresAtMs-s.PublishedAtMs > db.maxTTLms {
		return ErrBadRecord
	}
	pub, err := ids.IdentityPublicKey(s.OwnerIdentityID)
	if err != nil {
		return ErrBadRecord
	}
	unsigned, err := unsignedLeaseSetBytes(s)
	if err != nil {
		return ErrBadRecord
	}
	if !ed25519.Verify(pub, unsigned, s.Sig) {
		return ErrSigInvalid
	}
	return nil
}

func (db *DB) validateServiceHead(h ServiceHead, nowMs int64) error {
	if h.V != 1 {
		return ErrBadRecord
	}
	if err := ids.ValidateServiceName(h.ServiceName); err != nil {
		return ErrBadRecord
	}
	if err := ids.ValidateServiceID(h.ServiceID); err != nil {
		return ErrBadRecord
	}
	svcID, err := ids.NewServiceID(h.OwnerIdentityID, h.ServiceName)
	if err != nil || svcID != h.ServiceID {
		return ErrBadRecord
	}
	if h.ExpiresAtMs <= h.PublishedAtMs {
		return ErrBadRecord
	}
	if h.ExpiresAtMs <= nowMs {
		return ErrExpired
	}
	if h.ExpiresAtMs-h.PublishedAtMs > db.maxTTLms {
		return ErrBadRecord
	}
	if _, err := cid.Decode(h.DescriptorCID); err != nil {
		return ErrBadRecord
	}
	pub, err := ids.IdentityPublicKey(h.OwnerIdentityID)
	if err != nil {
		return ErrBadRecord
	}
	unsigned, err := unsignedServiceHeadBytes(h)
	if err != nil {
		return ErrBadRecord
	}
	if !ed25519.Verify(pub, unsigned, h.Sig) {
		return ErrSigInvalid
	}
	return nil
}

func unsignedRouterBytes(r RouterInfo) ([]byte, error) {
	return routerinfo.UnsignedBytes(
		r.V,
		r.PeerID,
		r.TransportPub,
		r.OnionPub,
		r.Addrs,
		r.Caps.Relay,
		r.Caps.NetDB,
		r.PublishedAtMs,
		r.ExpiresAtMs,
	)
}

func EncodeRouterInfo(r RouterInfo) ([]byte, error) { return codec.Marshal(r) }

func EncodeLeaseSet(s LeaseSet) ([]byte, error) { return codec.Marshal(s) }

func EncodeServiceHead(h ServiceHead) ([]byte, error) { return codec.Marshal(h) }

func SignRouterInfo(priv ed25519.PrivateKey, r RouterInfo) (RouterInfo, error) {
	unsigned, err := unsignedRouterBytes(r)
	if err != nil {
		return RouterInfo{}, err
	}
	r.Sig = ed25519.Sign(priv, unsigned)
	return r, nil
}

func SignLeaseSet(priv ed25519.PrivateKey, s LeaseSet) (LeaseSet, error) {
	unsigned, err := unsignedLeaseSetBytes(s)
	if err != nil {
		return LeaseSet{}, err
	}
	s.Sig = ed25519.Sign(priv, unsigned)
	return s, nil
}

func SignServiceHead(priv ed25519.PrivateKey, h ServiceHead) (ServiceHead, error) {
	unsigned, err := unsignedServiceHeadBytes(h)
	if err != nil {
		return ServiceHead{}, err
	}
	h.Sig = ed25519.Sign(priv, unsigned)
	return h, nil
}

func unsignedLeaseSetBytes(s LeaseSet) ([]byte, error) {
	type unsigned struct {
		V               uint64  `cbor:"v"`
		ServiceID       string  `cbor:"service_id"`
		OwnerIdentityID string  `cbor:"owner_identity_id"`
		ServiceName     string  `cbor:"service_name"`
		EncPub          []byte  `cbor:"enc_pub"`
		Leases          []Lease `cbor:"leases"`
		PublishedAtMs   int64   `cbor:"published_at_ms"`
		ExpiresAtMs     int64   `cbor:"expires_at_ms"`
	}
	u := unsigned{
		V:               s.V,
		ServiceID:       s.ServiceID,
		OwnerIdentityID: s.OwnerIdentityID,
		ServiceName:     s.ServiceName,
		EncPub:          s.EncPub,
		Leases:          s.Leases,
		PublishedAtMs:   s.PublishedAtMs,
		ExpiresAtMs:     s.ExpiresAtMs,
	}
	return codec.Marshal(u)
}

func unsignedServiceHeadBytes(h ServiceHead) ([]byte, error) {
	type unsigned struct {
		V               uint64 `cbor:"v"`
		ServiceID       string `cbor:"service_id"`
		OwnerIdentityID string `cbor:"owner_identity_id"`
		ServiceName     string `cbor:"service_name"`
		DescriptorCID   string `cbor:"descriptor_cid"`
		PublishedAtMs   int64  `cbor:"published_at_ms"`
		ExpiresAtMs     int64  `cbor:"expires_at_ms"`
	}
	u := unsigned{
		V:               h.V,
		ServiceID:       h.ServiceID,
		OwnerIdentityID: h.OwnerIdentityID,
		ServiceName:     h.ServiceName,
		DescriptorCID:   h.DescriptorCID,
		PublishedAtMs:   h.PublishedAtMs,
		ExpiresAtMs:     h.ExpiresAtMs,
	}
	return codec.Marshal(u)
}
