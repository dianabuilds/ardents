package runtime

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

func TestServicePublishDescriptorUpdatesAfterRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARDENTS_HOME", home)

	cfg := config.Default()
	cfg.Observability.HealthAddr = freeAddr(t)
	cfg.Observability.MetricsAddr = freeAddr(t)

	rt1 := New(cfg)
	setInboundLease(t, rt1)
	serviceName := "demo.msg.v1"
	serviceID, err := ids.NewServiceID(rt1.identity.ID, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	rt1.registerLocalService(localServiceInfo{
		ServiceID:   serviceID,
		ServiceName: serviceName,
	})
	if err := rt1.publishServiceHeadAndLeaseSet(localServiceInfo{ServiceID: serviceID, ServiceName: serviceName}); err != nil {
		t.Fatal(err)
	}
	head1, ok := rt1.netdb.ServiceHead(serviceID, timeutil.NowUnixMs())
	if !ok || head1.DescriptorCID == "" {
		t.Fatal("expected service head after first publish")
	}

	rt2 := New(cfg)
	rt2.netdb = rt1.netdb
	setInboundLease(t, rt2)
	rt2.registerLocalService(localServiceInfo{
		ServiceID:   serviceID,
		ServiceName: serviceName,
	})
	if err := rt2.publishServiceHeadAndLeaseSet(localServiceInfo{ServiceID: serviceID, ServiceName: serviceName}); err != nil {
		t.Fatal(err)
	}
	head2, ok := rt2.netdb.ServiceHead(serviceID, timeutil.NowUnixMs())
	if !ok || head2.DescriptorCID == "" {
		t.Fatal("expected service head after restart publish")
	}
	if head2.DescriptorCID == head1.DescriptorCID {
		t.Fatal("expected descriptor CID to change after restart")
	}
}

func setInboundLease(t *testing.T, rt *Runtime) {
	t.Helper()
	if rt == nil {
		t.Fatal("runtime is nil")
	}
	nowMs := timeutil.NowUnixMs()
	tunnelID := make([]byte, 16)
	if _, err := rand.Read(tunnelID); err != nil {
		t.Fatal(err)
	}
	pub := make([]byte, 32)
	if _, err := rand.Read(pub); err != nil {
		t.Fatal(err)
	}
	gatewayID, err := ids.NewPeerID(pub)
	if err != nil {
		t.Fatal(err)
	}
	path := &tunnelPath{
		direction:   "inbound",
		createdAtMs: nowMs,
		expiresAtMs: nowMs + int64((10*time.Minute)/time.Millisecond),
		hops: []tunnelHop{
			{peerID: gatewayID, tunnelID: tunnelID},
		},
	}
	rt.tunnelMgrMu.Lock()
	rt.inboundTunnels = []*tunnelPath{path}
	rt.tunnelMgrMu.Unlock()
}
