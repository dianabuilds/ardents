package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/domain/garlic"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type DeliveryTarget struct {
	PeerID    string
	ServiceID string
}

func (r *Runtime) deliverEnvelope(ctx context.Context, target DeliveryTarget, envBytes []byte) ([]byte, error) {
	if len(envBytes) == 0 {
		return nil, nil
	}
	if target.PeerID != "" {
		return directDeliverer{r: r}.Deliver(ctx, target, envBytes)
	}
	if target.ServiceID != "" {
		return tunnelDeliverer{r: r}.Deliver(ctx, target, envBytes)
	}
	return nil, nil
}

type directDeliverer struct {
	r *Runtime
}

func (d directDeliverer) Deliver(ctx context.Context, target DeliveryTarget, envBytes []byte) ([]byte, error) {
	if d.r == nil || target.PeerID == "" {
		return nil, errors.New("ERR_DELIVERY_TARGET_INVALID")
	}
	return d.r.deliverDirect(ctx, target.PeerID, envBytes)
}

type tunnelDeliverer struct {
	r *Runtime
}

func (d tunnelDeliverer) Deliver(ctx context.Context, target DeliveryTarget, envBytes []byte) ([]byte, error) {
	if d.r == nil || target.ServiceID == "" || len(envBytes) == 0 {
		return nil, nil
	}
	if d.r.netdb == nil {
		return nil, errors.New("ERR_NETDB_EMPTY")
	}
	leaseSet, ok := d.r.netdb.LeaseSet(target.ServiceID, timeutil.NowUnixMs())
	if !ok {
		return nil, errors.New("ERR_LEASESET_NOT_FOUND")
	}
	lease, ok := pickLease(leaseSet, timeutil.NowUnixMs())
	if !ok {
		return nil, errors.New("ERR_LEASE_EXPIRED")
	}
	out := d.r.pickOutboundTunnel()
	if out == nil {
		return nil, errors.New("ERR_TUNNEL_OUTBOUND_MISSING")
	}
	inner := garlic.Inner{
		V:           garlic.Version,
		ExpiresAtMs: minInt64(timeutil.NowUnixMs()+int64((1*time.Minute)/time.Millisecond), lease.ExpiresAtMs),
		Cloves: []garlic.Clove{
			{Kind: "envelope", Envelope: envBytes},
		},
	}
	msg, err := garlic.Encrypt(target.ServiceID, leaseSet.EncPub, inner)
	if err != nil {
		return nil, err
	}
	msgBytes, err := garlic.Encode(msg)
	if err != nil {
		return nil, err
	}
	dataBytes, err := d.r.buildTunnelData(out, tunnel.Inner{V: 1, Kind: "deliver", Garlic: msgBytes})
	if err != nil {
		return nil, err
	}
	entry := out.hops[0]
	env := d.r.buildTunnelDataEnvelope(entry.peerID, dataBytes)
	_, err = d.r.deliverDirect(ctx, entry.peerID, env)
	return nil, err
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
