package runtime

import (
	"errors"

	"github.com/dianabuilds/ardents/internal/core/transport/quic"
)

func (r *Runtime) observeHandshakeError(peerID string, remoteAddr string, err error) {
	_ = remoteAddr
	if r == nil {
		return
	}
	if err != nil && (errors.Is(err, quic.ErrMaxInboundConns) || errors.Is(err, quic.ErrHandshakeRateLimited)) {
		r.net.AddDegradedReason("transport_errors")
		r.log.Event("warn", "net", "net.degraded", "", "", "transport_errors")
	}
	r.handleHandshakeAbuse(peerID, err)
}
