package runtime

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/services/servicedesc"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/lockeys"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type localServiceInfo struct {
	ServiceID       string
	ServiceName     string
	DescriptorV1CID string
	DescriptorV2CID string
}

func (r *Runtime) registerLocalService(info localServiceInfo) {
	if info.ServiceID == "" || info.ServiceName == "" {
		return
	}
	r.localSvcMu.Lock()
	existing := r.localServices[info.ServiceID]
	if existing.ServiceID == "" {
		existing.ServiceID = info.ServiceID
	}
	if existing.ServiceName == "" {
		existing.ServiceName = info.ServiceName
	}
	if info.DescriptorV1CID != "" {
		existing.DescriptorV1CID = info.DescriptorV1CID
	}
	if info.DescriptorV2CID != "" {
		existing.DescriptorV2CID = info.DescriptorV2CID
	}
	r.localServices[info.ServiceID] = existing
	r.localSvcMu.Unlock()
}

func (r *Runtime) publishLocalServices() {
	if r == nil {
		return
	}
	r.localSvcMu.Lock()
	list := make([]localServiceInfo, 0, len(r.localServices))
	for _, v := range r.localServices {
		list = append(list, v)
	}
	r.localSvcMu.Unlock()
	for _, svc := range list {
		if err := r.publishServiceHeadAndLeaseSet(svc); err != nil {
			r.log.Event("warn", "service", "service.netdb.publish_failed", svc.ServiceID, svc.DescriptorV2CID, err.Error())
		}
	}
}

func (r *Runtime) startServicePublishTicker(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.publishLocalServices()
			}
		}
	}()
}

func (r *Runtime) publishServiceHeadAndLeaseSet(info localServiceInfo) error {
	if r.netdb == nil || r.identity.ID == "" || r.identity.PrivateKey == nil {
		return errors.New("ERR_SERVICE_PUBLISH_DISABLED")
	}
	if err := r.ensureServiceID(&info); err != nil {
		return err
	}
	descriptorCID, err := r.ensureServiceDescriptorV2(info)
	if err != nil {
		return err
	}
	nowMs := timeutil.NowUnixMs()
	leaseTTL := r.defaultLeaseTTL()
	maxTTL := r.netdbRecordMaxTTL()
	leaseExpires := clampExpiry(nowMs, leaseTTL, maxTTL)
	leases := r.buildInboundLeases(leaseExpires)
	if len(leases) == 0 {
		return errors.New("ERR_LEASES_EMPTY")
	}
	keypair, err := r.loadServiceKeys(info.ServiceID)
	if err != nil {
		return err
	}
	minLease := minLeaseExpiry(leases)
	headExpires := clampExpiry(nowMs, 3_600_000, maxTTL)
	if r.shouldSkipServicePublish(info.ServiceID, descriptorCID, leases, keypair.Public, minLease, headExpires, nowMs, leaseTTL) {
		return nil
	}
	if err := r.publishLeaseSet(info, leases, keypair.Public, minLease, nowMs); err != nil {
		return err
	}
	if err := r.publishServiceHead(info, descriptorCID, headExpires, nowMs); err != nil {
		return err
	}
	r.log.Event("info", "service", "service.netdb.published", info.ServiceID, descriptorCID, "")
	return nil
}

func (r *Runtime) buildInboundLeases(expiresAtMs int64) []netdb.Lease {
	nowMs := timeutil.NowUnixMs()
	r.tunnelMgrMu.Lock()
	defer r.tunnelMgrMu.Unlock()
	out := make([]netdb.Lease, 0, len(r.inboundTunnels))
	for _, t := range r.inboundTunnels {
		if len(t.hops) == 0 {
			continue
		}
		entry := t.hops[0]
		if entry.peerID == "" || len(entry.tunnelID) != 16 {
			continue
		}
		leaseExpires := t.expiresAtMs
		if leaseExpires <= nowMs {
			continue
		}
		if expiresAtMs > 0 && leaseExpires > expiresAtMs {
			leaseExpires = expiresAtMs
		}
		out = append(out, netdb.Lease{
			GatewayPeerID: entry.peerID,
			TunnelID:      append([]byte(nil), entry.tunnelID...),
			ExpiresAtMs:   leaseExpires,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ExpiresAtMs == out[j].ExpiresAtMs {
			if out[i].GatewayPeerID == out[j].GatewayPeerID {
				return string(out[i].TunnelID) < string(out[j].TunnelID)
			}
			return out[i].GatewayPeerID < out[j].GatewayPeerID
		}
		return out[i].ExpiresAtMs > out[j].ExpiresAtMs
	})
	if len(out) > netdb.MaxLeasesPerLeaseSet {
		out = out[:netdb.MaxLeasesPerLeaseSet]
	}
	return out
}

func (r *Runtime) ensureServiceDescriptorV2(info localServiceInfo) (string, error) {
	if info.ServiceID == "" || info.ServiceName == "" {
		return "", errors.New("ERR_SERVICE_DESCRIPTOR_INVALID")
	}
	if info.DescriptorV2CID != "" && r.store != nil {
		if _, err := r.store.Get(info.DescriptorV2CID); err == nil {
			return info.DescriptorV2CID, nil
		}
	}
	limits := map[string]uint64{
		"max_concurrency":   1,
		"max_payload_bytes": r.cfg.Limits.MaxPayloadBytes,
	}
	resources := map[string]uint64{
		"cpu_cores": uint64(runtime.NumCPU()),
		"ram_mb":    0,
	}
	caps := []servicedesc.Capability{{
		V:       1,
		JobType: info.ServiceName,
		Modes:   []string{},
	}}
	node, nodeID, err := servicedesc.BuildDescriptorNodeV2(r.identity.ID, r.identity.PrivateKey, info.ServiceName, caps, limits, resources)
	if err != nil {
		return "", err
	}
	nodeBytes, _, err := contentnode.EncodeWithCID(node)
	if err != nil {
		return "", err
	}
	if err := contentnode.VerifyBytes(nodeBytes, nodeID); err != nil {
		return "", err
	}
	if r.store != nil {
		if err := r.store.Put(nodeID, nodeBytes); err != nil {
			return "", err
		}
	}
	r.localSvcMu.Lock()
	if existing, ok := r.localServices[info.ServiceID]; ok {
		existing.DescriptorV2CID = nodeID
		r.localServices[info.ServiceID] = existing
	}
	r.localSvcMu.Unlock()
	return nodeID, nil
}

func (r *Runtime) ensureServiceID(info *localServiceInfo) error {
	expectedID, err := ids.NewServiceID(r.identity.ID, info.ServiceName)
	if err != nil {
		return err
	}
	if info.ServiceID != "" && info.ServiceID == expectedID {
		return nil
	}
	info.ServiceID = expectedID
	r.localSvcMu.Lock()
	if existing, ok := r.localServices[info.ServiceID]; ok {
		existing.ServiceID = expectedID
		r.localServices[expectedID] = existing
	}
	r.localSvcMu.Unlock()
	return nil
}

func (r *Runtime) defaultLeaseTTL() int64 {
	leaseTTL := r.tunnelParams().LeaseTTLMs
	if leaseTTL <= 0 {
		leaseTTL = 600_000
	}
	return leaseTTL
}

func (r *Runtime) netdbRecordMaxTTL() int64 {
	maxTTL := r.reseedParams.NetDB.RecordMaxTTLMs
	if maxTTL <= 0 {
		maxTTL = netdb.DefaultRecordMaxTTLMs
	}
	return maxTTL
}

func clampExpiry(nowMs int64, ttlMs int64, maxTTL int64) int64 {
	expires := nowMs + ttlMs
	if expires-nowMs > maxTTL {
		return nowMs + maxTTL
	}
	return expires
}

func (r *Runtime) loadServiceKeys(serviceID string) (lockeys.Keypair, error) {
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return lockeys.Keypair{}, err
	}
	return lockeys.LoadOrCreate(dirs.LKeysDir(), serviceID)
}

func minLeaseExpiry(leases []netdb.Lease) int64 {
	minLease := leases[0].ExpiresAtMs
	for _, l := range leases {
		if l.ExpiresAtMs < minLease {
			minLease = l.ExpiresAtMs
		}
	}
	return minLease
}

func (r *Runtime) publishLeaseSet(info localServiceInfo, leases []netdb.Lease, encPub []byte, minLease int64, nowMs int64) error {
	leaseSet := netdb.LeaseSet{
		V:               1,
		ServiceID:       info.ServiceID,
		OwnerIdentityID: r.identity.ID,
		ServiceName:     info.ServiceName,
		EncPub:          encPub,
		Leases:          leases,
		PublishedAtMs:   nowMs,
		ExpiresAtMs:     minLease,
	}
	leaseSet, err := netdb.SignLeaseSet(r.identity.PrivateKey, leaseSet)
	if err != nil {
		return err
	}
	leaseBytes, err := netdb.EncodeLeaseSet(leaseSet)
	if err != nil {
		return err
	}
	return r.storeNetDBRecord(nowMs, leaseBytes)
}

func (r *Runtime) publishServiceHead(info localServiceInfo, descriptorCID string, headExpires int64, nowMs int64) error {
	head := netdb.ServiceHead{
		V:               1,
		ServiceID:       info.ServiceID,
		OwnerIdentityID: r.identity.ID,
		ServiceName:     info.ServiceName,
		DescriptorCID:   descriptorCID,
		PublishedAtMs:   nowMs,
		ExpiresAtMs:     headExpires,
	}
	head, err := netdb.SignServiceHead(r.identity.PrivateKey, head)
	if err != nil {
		return err
	}
	headBytes, err := netdb.EncodeServiceHead(head)
	if err != nil {
		return err
	}
	return r.storeNetDBRecord(nowMs, headBytes)
}

func (r *Runtime) storeNetDBRecord(nowMs int64, recordBytes []byte) error {
	if r.netdb == nil {
		return errors.New("ERR_NETDB_UNAVAILABLE")
	}
	if status, code := r.netdb.Store(recordBytes, nowMs); status != "OK" {
		return errors.New(code)
	}
	return nil
}

func (r *Runtime) shouldSkipServicePublish(serviceID string, descriptorCID string, leases []netdb.Lease, encPub []byte, leaseExpiresAtMs int64, headExpiresAtMs int64, nowMs int64, leaseTTL int64) bool {
	if r.netdb == nil || serviceID == "" {
		return false
	}
	currentLease, okLease := r.netdb.LeaseSet(serviceID, nowMs)
	currentHead, okHead := r.netdb.ServiceHead(serviceID, nowMs)
	if !okLease || !okHead {
		return false
	}
	if currentHead.DescriptorCID != descriptorCID {
		return false
	}
	refreshAt := leaseTTL / 2
	if refreshAt <= 0 {
		refreshAt = 300_000
	}
	if currentLease.ExpiresAtMs-nowMs <= refreshAt || currentHead.ExpiresAtMs-nowMs <= refreshAt {
		return false
	}
	if currentLease.ExpiresAtMs != leaseExpiresAtMs || currentHead.ExpiresAtMs != headExpiresAtMs {
		return false
	}
	if !bytesEqual(currentLease.EncPub, encPub) {
		return false
	}
	if !leasesEqual(currentLease.Leases, leases) {
		return false
	}
	return true
}

func leasesEqual(a []netdb.Lease, b []netdb.Lease) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]netdb.Lease(nil), a...)
	bb := append([]netdb.Lease(nil), b...)
	sortLeases(aa)
	sortLeases(bb)
	for i := range aa {
		if aa[i].GatewayPeerID != bb[i].GatewayPeerID {
			return false
		}
		if aa[i].ExpiresAtMs != bb[i].ExpiresAtMs {
			return false
		}
		if !bytesEqual(aa[i].TunnelID, bb[i].TunnelID) {
			return false
		}
	}
	return true
}

func sortLeases(leases []netdb.Lease) {
	sort.Slice(leases, func(i, j int) bool {
		if leases[i].GatewayPeerID == leases[j].GatewayPeerID {
			if leases[i].ExpiresAtMs == leases[j].ExpiresAtMs {
				return string(leases[i].TunnelID) < string(leases[j].TunnelID)
			}
			return leases[i].ExpiresAtMs < leases[j].ExpiresAtMs
		}
		return leases[i].GatewayPeerID < leases[j].GatewayPeerID
	})
}
