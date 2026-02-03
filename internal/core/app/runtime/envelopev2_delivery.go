package runtime

import (
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/domain/garlic"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func (r *Runtime) deliverEnvelopeV2(replyTo *envelopev2.Reply, envBytes []byte) error {
	if replyTo == nil || replyTo.ServiceID == "" || len(envBytes) == 0 {
		return nil
	}
	if r.netdb == nil {
		return errors.New("ERR_NETDB_EMPTY")
	}
	leaseSet, ok := r.netdb.LeaseSet(replyTo.ServiceID, timeutil.NowUnixMs())
	if !ok {
		return errors.New("ERR_LEASESET_NOT_FOUND")
	}
	lease, ok := pickLease(leaseSet, timeutil.NowUnixMs())
	if !ok {
		return errors.New("ERR_LEASE_EXPIRED")
	}
	out := r.pickOutboundTunnel()
	if out == nil {
		return errors.New("ERR_TUNNEL_OUTBOUND_MISSING")
	}
	inner := garlic.Inner{
		V:           garlic.Version,
		ExpiresAtMs: minInt64(timeutil.NowUnixMs()+int64((1*time.Minute)/time.Millisecond), lease.ExpiresAtMs),
		Cloves: []garlic.Clove{
			{Kind: "envelope", Envelope: envBytes},
		},
	}
	msg, err := garlic.Encrypt(replyTo.ServiceID, leaseSet.EncPub, inner)
	if err != nil {
		return err
	}
	msgBytes, err := garlic.Encode(msg)
	if err != nil {
		return err
	}
	dataBytes, err := r.buildTunnelData(out, tunnel.Inner{V: 1, Kind: "deliver", Garlic: msgBytes})
	if err != nil {
		return err
	}
	entry := out.hops[0]
	env := r.buildTunnelDataEnvelope(entry.peerID, dataBytes)
	_, err = r.forwardEnvelope(entry.peerID, env)
	return err
}

func (r *Runtime) pickOutboundTunnel() *tunnelPath {
	r.tunnelMgrMu.Lock()
	defer r.tunnelMgrMu.Unlock()
	if len(r.outboundTunnels) == 0 {
		return nil
	}
	return r.outboundTunnels[0]
}

func pickLease(set netdb.LeaseSet, nowMs int64) (netdb.Lease, bool) {
	if len(set.Leases) == 0 {
		return netdb.Lease{}, false
	}
	var best netdb.Lease
	ok := false
	for _, l := range set.Leases {
		if l.ExpiresAtMs <= nowMs {
			continue
		}
		if !ok || l.ExpiresAtMs > best.ExpiresAtMs {
			best = l
			ok = true
		}
	}
	return best, ok
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
