//go:build real_waku

package waku

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGoWakuMessageExchangeAndStoreRetrieval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	nodeA := startRealWakuNode(t, ctx, "aim1alice", nil, true)

	bootstrap := firstLoopbackAddr(nodeA.ListenAddresses())
	if bootstrap == "" {
		t.Skip("no loopback listen address for node A")
	}

	nodeB1 := startRealWakuNode(t, ctx, "aim1bob", []string{bootstrap}, false)

	msgCh := make(chan PrivateMessage, 4)
	if err := nodeB1.SubscribePrivate(func(msg PrivateMessage) {
		msgCh <- msg
	}); err != nil {
		t.Fatalf("node B subscribe failed: %v", err)
	}

	onlineMsg := PrivateMessage{
		ID:        "realwaku_online_1",
		SenderID:  "aim1alice",
		Recipient: "aim1bob",
		Payload:   []byte("hello-over-relay"),
	}
	if err := nodeA.PublishPrivate(ctx, onlineMsg); err != nil {
		t.Fatalf("publish online message failed: %v", err)
	}

	select {
	case got := <-msgCh:
		if got.ID != onlineMsg.ID {
			t.Fatalf("unexpected online msg id: %s", got.ID)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("timed out waiting for online message via relay")
	}

	if err := nodeB1.Stop(context.Background()); err != nil {
		t.Fatalf("stop node B failed: %v", err)
	}

	since := time.Now().Add(-2 * time.Second)
	offlineMsg := PrivateMessage{
		ID:        "realwaku_offline_1",
		SenderID:  "aim1alice",
		Recipient: "aim1bob",
		Payload:   []byte("hello-from-store"),
	}
	if err := nodeA.PublishPrivate(ctx, offlineMsg); err != nil {
		t.Fatalf("publish offline message failed: %v", err)
	}

	nodeB2 := startRealWakuNode(t, ctx, "aim1bob", []string{bootstrap}, false)

	missed, err := nodeB2.FetchPrivateSince(ctx, "aim1bob", since, 200)
	if err != nil {
		t.Fatalf("fetch missed messages failed: %v", err)
	}
	found := false
	for _, got := range missed {
		if got.ID == offlineMsg.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("offline message %q was not recovered via store path", offlineMsg.ID)
	}
}

func TestGoWakuFailoverWithFirstBootstrapDown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	nodeA := startRealWakuNode(t, ctx, "aim1alice", nil, true)
	bootstrapA := firstLoopbackAddr(nodeA.ListenAddresses())
	if bootstrapA == "" {
		t.Skip("no loopback listen address for node A")
	}

	// Keep an additional alive bootstrap for redundancy.
	nodeC := startRealWakuNode(t, ctx, "aim1charlie", []string{bootstrapA}, true)
	bootstrapC := firstLoopbackAddr(nodeC.ListenAddresses())
	if bootstrapC == "" {
		t.Skip("no loopback listen address for node C")
	}

	// First bootstrap address is captured from a node that is then stopped.
	cfgDead := DefaultConfig()
	cfgDead.Transport = TransportGoWaku
	cfgDead.Port = 0
	nodeDead := NewNode(cfgDead)
	if err := nodeDead.Start(ctx); err != nil {
		t.Fatalf("start dead node failed: %v", err)
	}
	deadBootstrap := firstLoopbackAddr(nodeDead.ListenAddresses())
	if deadBootstrap == "" {
		t.Skip("no loopback listen address for dead bootstrap node")
	}
	if err := nodeDead.Stop(context.Background()); err != nil {
		t.Fatalf("stop dead bootstrap node failed: %v", err)
	}

	cfgBBootstrap := []string{deadBootstrap, bootstrapA, bootstrapC}

	nodeB1 := startRealWakuNode(t, ctx, "aim1bob", cfgBBootstrap, false)
	waitForPeerCountAtLeast(t, nodeB1, 1, 10*time.Second)

	msgCh := make(chan PrivateMessage, 4)
	if err := nodeB1.SubscribePrivate(func(msg PrivateMessage) {
		msgCh <- msg
	}); err != nil {
		t.Fatalf("node B subscribe failed: %v", err)
	}

	onlineMsg := PrivateMessage{
		ID:        "realwaku_failover_online_1",
		SenderID:  "aim1alice",
		Recipient: "aim1bob",
		Payload:   []byte("online-via-secondary-bootstrap"),
	}
	if err := nodeA.PublishPrivate(ctx, onlineMsg); err != nil {
		t.Fatalf("publish online message failed: %v", err)
	}
	select {
	case got := <-msgCh:
		if got.ID != onlineMsg.ID {
			t.Fatalf("unexpected online message id: %s", got.ID)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("timed out waiting for online failover message")
	}

	if err := nodeB1.Stop(context.Background()); err != nil {
		t.Fatalf("stop node B failed: %v", err)
	}

	since := time.Now().Add(-2 * time.Second)
	offlineMsg := PrivateMessage{
		ID:        "realwaku_failover_offline_1",
		SenderID:  "aim1alice",
		Recipient: "aim1bob",
		Payload:   []byte("offline-via-store-failover"),
	}
	if err := nodeA.PublishPrivate(ctx, offlineMsg); err != nil {
		t.Fatalf("publish offline message failed: %v", err)
	}

	nodeB2 := startRealWakuNode(t, ctx, "aim1bob", cfgBBootstrap, false)

	missed, err := nodeB2.FetchPrivateSince(ctx, "aim1bob", since, 200)
	if err != nil {
		t.Fatalf("fetch missed messages with failed primary bootstrap failed: %v", err)
	}
	found := false
	for _, got := range missed {
		if got.ID == offlineMsg.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("offline message %q was not recovered with first bootstrap down", offlineMsg.ID)
	}
}

func waitForPeerCountAtLeast(t *testing.T, n *Node, minPeers int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if n.Status().PeerCount >= minPeers {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for peer count >= %d, got %d", minPeers, n.Status().PeerCount)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func startRealWakuNode(t *testing.T, ctx context.Context, identity string, bootstrapNodes []string, subscribe bool) *Node {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Transport = TransportGoWaku
	cfg.Port = 0
	cfg.BootstrapNodes = append([]string(nil), bootstrapNodes...)
	node := NewNode(cfg)
	if err := node.Start(ctx); err != nil {
		t.Fatalf("start node %s failed: %v", identity, err)
	}
	t.Cleanup(func() { _ = node.Stop(context.Background()) })
	node.SetIdentity(identity)
	if subscribe {
		if err := node.SubscribePrivate(func(PrivateMessage) {}); err != nil {
			t.Fatalf("node %s subscribe failed: %v", identity, err)
		}
	}
	return node
}

func firstLoopbackAddr(addrs []string) string {
	for _, addr := range addrs {
		if strings.Contains(addr, "/p2p/") && strings.Contains(addr, "/tcp/") && strings.Contains(addr, "/127.0.0.1/") {
			return addr
		}
	}
	for _, addr := range addrs {
		if strings.Contains(addr, "/p2p/") && strings.Contains(addr, "/tcp/") {
			return addr
		}
	}
	return ""
}
