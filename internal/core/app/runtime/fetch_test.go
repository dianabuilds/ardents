package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/domain/providers"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
)

func TestFetchNodeFromProvider(t *testing.T) {
	providerCfg := config.Default()
	providerCfg.Listen.QUICAddr = "127.0.0.1:0"
	providerCfg.Observability.HealthAddr = freeAddr(t)
	providerCfg.Observability.MetricsAddr = freeAddr(t)
	provider := New(providerCfg)
	if err := provider.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = provider.Stop(context.Background())
	})

	node := contentnode.Node{
		V:           1,
		Type:        "test.node.v1",
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Owner:       provider.IdentityID(),
		Links:       []contentnode.Link{},
		Body: map[string]any{
			"v":     uint64(1),
			"value": "ok",
		},
		Policy: map[string]any{
			"v":          uint64(1),
			"visibility": "public",
		},
	}
	if err := contentnode.Sign(&node, provider.identity.PrivateKey); err != nil {
		t.Fatal(err)
	}
	nodeBytes, nodeID, err := contentnode.EncodeWithCID(node)
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.store.Put(nodeID, nodeBytes); err != nil {
		t.Fatal(err)
	}

	clientCfg := config.Default()
	clientCfg.Observability.HealthAddr = freeAddr(t)
	clientCfg.Observability.MetricsAddr = freeAddr(t)
	clientCfg.BootstrapPeers = []config.BootstrapPeer{
		{PeerID: provider.PeerID(), Addrs: []string{"quic://" + provider.QUICAddr()}},
	}
	client := New(clientCfg)
	nowMs := time.Now().UTC().UnixNano() / int64(time.Millisecond)
	client.providers.Add(providers.ProviderRecord{
		V:              1,
		NodeID:         nodeID,
		ProviderPeerID: provider.PeerID(),
		TSMs:           nowMs,
		TTLMs:          int64((5 * time.Minute) / time.Millisecond),
	}, nowMs)

	got, err := client.FetchNode(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(nodeBytes) {
		t.Fatalf("unexpected node bytes")
	}
	if cached, err := client.store.Get(nodeID); err != nil || string(cached) != string(nodeBytes) {
		t.Fatalf("expected cache entry")
	}
}
