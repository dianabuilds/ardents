package quic

import (
	"crypto/ed25519"
	"crypto/x509"
	"errors"

	"github.com/dianabuilds/ardents/internal/shared/ids"
)

var ErrPeerCertInvalid = errors.New("ERR_PEER_CERT_INVALID")

func PeerIDFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", ErrPeerCertInvalid
	}
	pub, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return "", ErrPeerCertInvalid
	}
	return ids.NewPeerID(pub)
}
