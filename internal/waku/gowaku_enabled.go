//go:build real_waku

package waku

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/waku-org/go-waku/waku/persistence"
	"github.com/waku-org/go-waku/waku/persistence/sqlite"
	wakuNode "github.com/waku-org/go-waku/waku/v2/node"
	"github.com/waku-org/go-waku/waku/v2/protocol"
	legacyStore "github.com/waku-org/go-waku/waku/v2/protocol/legacy_store"
	wpb "github.com/waku-org/go-waku/waku/v2/protocol/pb"
	"github.com/waku-org/go-waku/waku/v2/protocol/relay"
	"github.com/waku-org/go-waku/waku/v2/utils"
)

const (
	privatePubsubTopic  = "/waku/2/default-waku/proto"
	privateContentTopic = "/aim-chat/1/private-message/proto"
)

type goWakuNode struct {
	mu             sync.RWMutex
	node           *wakuNode.WakuNode
	selfID         string
	handler        func(PrivateMessage)
	cfg            Config
	bootstrapNodes []string
	maintainCancel context.CancelFunc
	maintainWG     sync.WaitGroup
	metrics        goWakuMetrics
}

type goWakuMetrics struct {
	DialAttempts       int
	DialSuccess        int
	DialFailures       int
	StoreQueryFailover int
	StoreQueryFailures int
}

func newGoWakuBackend() goWakuBackend {
	return &goWakuNode{}
}

func (g *goWakuNode) Start(ctx context.Context, cfg Config) error {
	opts := make([]wakuNode.WakuNodeOption, 0)
	hostAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(cfg.Port)))
	if err != nil {
		return err
	}
	opts = append(opts, wakuNode.WithHostAddress(hostAddr))
	if cfg.EnableRelay {
		opts = append(opts, wakuNode.WithWakuRelay())
	}
	if cfg.EnableStore {
		provider, err := newInMemoryMessageProvider()
		if err != nil {
			return err
		}
		opts = append(opts, wakuNode.WithMessageProvider(provider))
		opts = append(opts, wakuNode.WithWakuStore())
	}
	if cfg.EnableFilter {
		opts = append(opts, wakuNode.WithWakuFilterLightNode(), wakuNode.WithWakuFilterFullNode())
	}
	if cfg.EnableLightPush {
		opts = append(opts, wakuNode.WithLightPush())
	}

	node, err := wakuNode.New(opts...)
	if err != nil {
		return err
	}
	if err := node.Start(ctx); err != nil {
		return err
	}

	for _, addr := range cfg.BootstrapNodes {
		_ = node.DialPeer(ctx, addr)
	}

	g.mu.Lock()
	g.node = node
	g.cfg = cfg
	g.bootstrapNodes = append([]string(nil), cfg.BootstrapNodes...)
	g.mu.Unlock()
	if cfg.FailoverV1 {
		g.startPeerMaintenance()
	}
	return nil
}

func (g *goWakuNode) Stop() {
	g.stopPeerMaintenance()

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.node != nil {
		g.node.Stop()
		g.node = nil
	}
}

func (g *goWakuNode) PeerCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.node == nil {
		return 0
	}
	return g.node.PeerCount()
}

func (g *goWakuNode) NetworkMetrics() map[string]int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return map[string]int{
		"dial_attempts":        g.metrics.DialAttempts,
		"dial_success":         g.metrics.DialSuccess,
		"dial_failures":        g.metrics.DialFailures,
		"store_query_failover": g.metrics.StoreQueryFailover,
		"store_query_failures": g.metrics.StoreQueryFailures,
	}
}

func (g *goWakuNode) ApplyConfig(cfg Config) {
	g.mu.Lock()
	g.cfg.MinPeers = cfg.MinPeers
	g.cfg.ReconnectInterval = cfg.ReconnectInterval
	g.cfg.ReconnectBackoffMax = cfg.ReconnectBackoffMax
	g.cfg.FailoverV1 = cfg.FailoverV1
	g.bootstrapNodes = append([]string(nil), cfg.BootstrapNodes...)
	g.mu.Unlock()

	if cfg.FailoverV1 {
		g.startPeerMaintenance()
		return
	}
	g.stopPeerMaintenance()
}

func (g *goWakuNode) SetIdentity(identityID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.selfID = identityID
}

func (g *goWakuNode) ListenAddresses() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.node == nil {
		return nil
	}
	addrs := g.node.ListenAddresses()
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		out = append(out, addr.String())
	}
	return out
}

func (g *goWakuNode) SubscribePrivate(handler func(PrivateMessage)) error {
	g.mu.Lock()
	g.handler = handler
	node := g.node
	selfID := g.selfID
	g.mu.Unlock()
	if node == nil {
		return errors.New("go-waku node is nil")
	}
	if selfID == "" {
		return errors.New("identity is not set")
	}

	filter := protocol.NewContentFilter(privatePubsubTopic, privateContentTopic)
	subs, err := node.Relay().Subscribe(context.Background(), filter)
	if err != nil {
		return err
	}

	for _, sub := range subs {
		go func(subscription *relay.Subscription) {
			for env := range subscription.Ch {
				if env == nil || env.Message() == nil {
					continue
				}
				var msg PrivateMessage
				if err := json.Unmarshal(env.Message().Payload, &msg); err != nil {
					continue
				}
				if msg.Recipient != selfID {
					continue
				}
				handler(msg)
			}
		}(sub)
	}

	return nil
}

func (g *goWakuNode) PublishPrivate(ctx context.Context, msg PrivateMessage) error {
	g.mu.RLock()
	node := g.node
	g.mu.RUnlock()
	if node == nil {
		return errors.New("go-waku node is nil")
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ts := time.Now().UnixNano()
	wm := &wpb.WakuMessage{
		Payload:      payload,
		ContentTopic: privateContentTopic,
		Timestamp:    &ts,
	}
	_, err = node.Relay().Publish(ctx, wm, relay.WithPubSubTopic(privatePubsubTopic))
	return err
}

func (g *goWakuNode) FetchPrivateSince(ctx context.Context, recipient string, since time.Time, limit int) ([]PrivateMessage, error) {
	g.mu.RLock()
	node := g.node
	g.mu.RUnlock()
	if node == nil {
		return nil, errors.New("go-waku node is nil")
	}
	if recipient == "" {
		return nil, errors.New("recipient is required")
	}
	if limit <= 0 {
		limit = 100
	}
	start := since.UnixNano()
	end := time.Now().UnixNano()
	criteria := legacyStore.Query{
		PubsubTopic:   privatePubsubTopic,
		ContentTopics: []string{privateContentTopic},
		StartTime:     &start,
		EndTime:       &end,
	}
	baseOpts := []legacyStore.HistoryRequestOption{legacyStore.WithPaging(true, uint64(limit))}
	g.mu.RLock()
	bootstrapNodes := append([]string(nil), g.bootstrapNodes...)
	fanout := g.cfg.StoreQueryFanout
	failoverEnabled := g.cfg.FailoverV1
	g.mu.RUnlock()
	if fanout <= 0 {
		fanout = 1
	}

	type queryCandidate struct {
		opts     []legacyStore.HistoryRequestOption
		peerAddr string
	}
	candidates := make([]queryCandidate, 0, minInt(len(bootstrapNodes), fanout)+1)
	seen := make(map[string]struct{}, len(bootstrapNodes))
	for _, addr := range bootstrapNodes {
		if len(candidates) >= fanout {
			break
		}
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		peerAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			continue
		}
		opts := append([]legacyStore.HistoryRequestOption{}, baseOpts...)
		opts = append(opts, legacyStore.WithPeerAddr(peerAddr))
		candidates = append(candidates, queryCandidate{opts: opts, peerAddr: addr})
	}
	// Last attempt without forcing peer address so go-waku can use available peers.
	candidates = append(candidates, queryCandidate{
		opts:     append([]legacyStore.HistoryRequestOption{}, baseOpts...),
		peerAddr: "auto",
	})

	var (
		result  *legacyStore.Result
		err     error
		lastErr error
	)
	successAttempt := 0
	if !failoverEnabled && len(candidates) > 0 {
		candidates = candidates[:1]
	}
	for i, candidate := range candidates {
		attempt := i + 1
		result, err = node.LegacyStore().Query(ctx, criteria, candidate.opts...)
		if err == nil {
			successAttempt = attempt
			break
		}
		g.recordStoreQueryFailure()
		slog.Warn("store query attempt failed", "peer_addr", candidate.peerAddr, "attempt", attempt, "reason", err.Error())
		lastErr = err
	}
	if err != nil {
		return nil, lastErr
	}
	if successAttempt > 1 {
		g.recordStoreQueryFailover()
		slog.Info("store query recovered via failover", "attempt", successAttempt)
	}

	msgByID := map[string]PrivateMessage{}
	order := make([]string, 0, limit)
	consume := func() {
		for _, wm := range result.Messages {
			if wm == nil {
				continue
			}
			var msg PrivateMessage
			if err := json.Unmarshal(wm.Payload, &msg); err != nil {
				continue
			}
			if msg.Recipient != recipient {
				continue
			}
			if _, exists := msgByID[msg.ID]; exists {
				continue
			}
			msgByID[msg.ID] = msg
			order = append(order, msg.ID)
		}
	}
	consume()
	for !result.IsComplete() && len(order) < limit {
		result, err = node.LegacyStore().Next(ctx, result)
		if err != nil {
			return nil, err
		}
		consume()
	}

	// Keep deterministic order by ID when store responses contain mixed peers/pages.
	sort.Strings(order)
	if len(order) > limit {
		order = order[:limit]
	}
	out := make([]PrivateMessage, 0, len(order))
	for _, id := range order {
		out = append(out, msgByID[id])
	}
	return out, nil
}

func (g *goWakuNode) startPeerMaintenance() {
	g.mu.Lock()
	if g.maintainCancel != nil {
		g.maintainCancel()
		g.maintainCancel = nil
	}
	if len(g.bootstrapNodes) == 0 || g.node == nil {
		g.mu.Unlock()
		return
	}
	maintainCtx, cancel := context.WithCancel(context.Background())
	g.maintainCancel = cancel
	g.maintainWG.Add(1)
	cfg := g.cfg
	g.mu.Unlock()

	go func() {
		defer g.maintainWG.Done()
		ticker := time.NewTicker(cfg.ReconnectInterval)
		defer ticker.Stop()

		backoff := cfg.ReconnectInterval
		nextAttemptAt := time.Now()
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

		for {
			select {
			case <-maintainCtx.Done():
				return
			case <-ticker.C:
				if time.Now().Before(nextAttemptAt) {
					continue
				}
				if !g.needMorePeers() {
					backoff = cfg.ReconnectInterval
					nextAttemptAt = time.Now()
					continue
				}

				ok := g.redialBootstrapPeers(maintainCtx, rnd)
				if ok || !g.needMorePeers() {
					backoff = cfg.ReconnectInterval
					nextAttemptAt = time.Now()
					continue
				}

				backoff *= 2
				if backoff > cfg.ReconnectBackoffMax {
					backoff = cfg.ReconnectBackoffMax
				}
				jitter := time.Duration(rnd.Int63n(int64(backoff / 2)))
				nextAttemptAt = time.Now().Add(backoff + jitter)
			}
		}
	}()
}

func (g *goWakuNode) stopPeerMaintenance() {
	g.mu.Lock()
	cancel := g.maintainCancel
	g.maintainCancel = nil
	g.mu.Unlock()
	if cancel != nil {
		cancel()
		g.maintainWG.Wait()
	}
}

func (g *goWakuNode) needMorePeers() bool {
	g.mu.RLock()
	node := g.node
	bootstrapCount := len(g.bootstrapNodes)
	target := g.cfg.MinPeers
	g.mu.RUnlock()
	if node == nil {
		return false
	}
	if target <= 0 {
		target = desiredPeerFloor(bootstrapCount)
	}
	if bootstrapCount > 0 && target > bootstrapCount {
		target = bootstrapCount
	}
	return node.PeerCount() < target
}

func desiredPeerFloor(bootstrapCount int) int {
	if bootstrapCount <= 0 {
		return 0
	}
	if bootstrapCount == 1 {
		return 1
	}
	return 2
}

func (g *goWakuNode) redialBootstrapPeers(ctx context.Context, rnd *rand.Rand) bool {
	g.mu.RLock()
	node := g.node
	bootstrapNodes := append([]string(nil), g.bootstrapNodes...)
	g.mu.RUnlock()
	if node == nil || len(bootstrapNodes) == 0 {
		return false
	}

	rnd.Shuffle(len(bootstrapNodes), func(i, j int) {
		bootstrapNodes[i], bootstrapNodes[j] = bootstrapNodes[j], bootstrapNodes[i]
	})

	success := false
	for i, addr := range bootstrapNodes {
		attempt := i + 1
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		g.recordDialAttempt()
		if err := node.DialPeer(ctx, addr); err == nil {
			g.recordDialSuccess()
			success = true
			slog.Info("peer redial succeeded", "peer_addr", addr, "attempt", attempt)
			continue
		} else {
			g.recordDialFailure()
			slog.Warn("peer redial failed", "peer_addr", addr, "attempt", attempt, "reason", err.Error())
		}
	}
	return success
}

func (g *goWakuNode) recordDialAttempt() {
	g.mu.Lock()
	g.metrics.DialAttempts++
	g.mu.Unlock()
}

func (g *goWakuNode) recordDialSuccess() {
	g.mu.Lock()
	g.metrics.DialSuccess++
	g.mu.Unlock()
}

func (g *goWakuNode) recordDialFailure() {
	g.mu.Lock()
	g.metrics.DialFailures++
	g.mu.Unlock()
}

func (g *goWakuNode) recordStoreQueryFailover() {
	g.mu.Lock()
	g.metrics.StoreQueryFailover++
	g.mu.Unlock()
}

func (g *goWakuNode) recordStoreQueryFailure() {
	g.mu.Lock()
	g.metrics.StoreQueryFailures++
	g.mu.Unlock()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func newInMemoryMessageProvider() (*persistence.DBStore, error) {
	db, err := sqlite.NewDB(":memory:", utils.Logger())
	if err != nil {
		return nil, err
	}
	return persistence.NewDBStore(
		prometheus.DefaultRegisterer,
		utils.Logger(),
		persistence.WithDB(db),
		persistence.WithMigrations(sqlite.Migrations),
	)
}
