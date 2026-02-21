package waku

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	TransportMock   = "mock"
	TransportGoWaku = "go-waku"

	StateDisconnected = "disconnected"
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateDegraded     = "degraded"
)

var runtimeStatusPollInterval = 1 * time.Second

type Config struct {
	Transport                  string        `yaml:"transport"`
	Port                       int           `yaml:"port"`
	AdvertiseAddress           string        `yaml:"advertiseAddress"`
	EnableRelay                bool          `yaml:"enableRelay"`
	EnableStore                bool          `yaml:"enableStore"`
	EnableFilter               bool          `yaml:"enableFilter"`
	EnableLightPush            bool          `yaml:"enableLightPush"`
	BootstrapNodes             []string      `yaml:"bootstrapNodes"`
	FailoverV1                 bool          `yaml:"failoverV1"`
	MinPeers                   int           `yaml:"minPeers"`
	StoreQueryFanout           int           `yaml:"storeQueryFanout"`
	ReconnectInterval          time.Duration `yaml:"reconnectInterval"`
	ReconnectBackoffMax        time.Duration `yaml:"reconnectBackoffMax"`
	ManifestRefreshInterval    time.Duration `yaml:"manifestRefreshInterval"`
	ManifestStaleWindow        time.Duration `yaml:"manifestStaleWindow"`
	ManifestRefreshTimeout     time.Duration `yaml:"manifestRefreshTimeout"`
	ManifestBackoffBase        time.Duration `yaml:"manifestBackoffBase"`
	ManifestBackoffMax         time.Duration `yaml:"manifestBackoffMax"`
	ManifestBackoffFactor      float64       `yaml:"manifestBackoffFactor"`
	ManifestBackoffJitterRatio float64       `yaml:"manifestBackoffJitterRatio"`
	BootstrapSource            string        `yaml:"-"`
	BootstrapManifestVersion   int           `yaml:"-"`
	BootstrapManifestKeyID     string        `yaml:"-"`
	BootstrapManifestPath      string        `yaml:"-"`
	BootstrapTrustBundlePath   string        `yaml:"-"`
	BootstrapCachePath         string        `yaml:"-"`
}

type Status struct {
	State                    string
	PeerCount                int
	LastSync                 time.Time
	BootstrapSource          string
	BootstrapManifestVersion int
	BootstrapManifestKeyID   string
}

type Node struct {
	mu      sync.RWMutex
	cfg     Config
	status  Status
	selfID  string
	handler func(PrivateMessage)
	gw      goWakuBackend

	monitorCancel    context.CancelFunc
	monitorWG        sync.WaitGroup
	stateTransitions int
}

type goWakuBackend interface {
	Start(ctx context.Context, cfg Config) error
	Stop()
	PeerCount() int
	NetworkMetrics() map[string]int
	ApplyConfig(cfg Config)
	SetIdentity(identityID string)
	ListenAddresses() []string
	SubscribePrivate(handler func(PrivateMessage)) error
	PublishPrivate(ctx context.Context, msg PrivateMessage) error
	FetchPrivateSince(ctx context.Context, recipient string, since time.Time, limit int) ([]PrivateMessage, error)
}

func DefaultConfig() Config {
	return Config{
		Transport:                  TransportMock,
		Port:                       60000,
		EnableRelay:                true,
		EnableStore:                true,
		EnableFilter:               true,
		EnableLightPush:            true,
		BootstrapNodes:             nil,
		FailoverV1:                 true,
		MinPeers:                   2,
		StoreQueryFanout:           3,
		ReconnectInterval:          1 * time.Second,
		ReconnectBackoffMax:        30 * time.Second,
		ManifestRefreshInterval:    60 * time.Second,
		ManifestStaleWindow:        5 * time.Minute,
		ManifestRefreshTimeout:     5 * time.Second,
		ManifestBackoffBase:        1 * time.Second,
		ManifestBackoffMax:         30 * time.Second,
		ManifestBackoffFactor:      2.0,
		ManifestBackoffJitterRatio: 0.2,
	}
}

func NewNode(cfg Config) *Node {
	cfg = normalizeConfig(cfg)
	return &Node{
		cfg: cfg,
		status: Status{
			State:           StateDisconnected,
			PeerCount:       0,
			BootstrapSource: cfg.BootstrapSource,
		},
	}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Transport == "" {
		cfg.Transport = def.Transport
	}
	if cfg.StoreQueryFanout <= 0 {
		cfg.StoreQueryFanout = def.StoreQueryFanout
	}
	if cfg.ReconnectInterval <= 0 {
		cfg.ReconnectInterval = def.ReconnectInterval
	}
	if cfg.ReconnectBackoffMax <= 0 {
		cfg.ReconnectBackoffMax = def.ReconnectBackoffMax
	}
	if cfg.ReconnectBackoffMax < cfg.ReconnectInterval {
		cfg.ReconnectBackoffMax = cfg.ReconnectInterval
	}
	if cfg.ManifestRefreshInterval <= 0 {
		cfg.ManifestRefreshInterval = def.ManifestRefreshInterval
	}
	if cfg.ManifestStaleWindow <= 0 {
		cfg.ManifestStaleWindow = def.ManifestStaleWindow
	}
	if cfg.ManifestRefreshTimeout <= 0 {
		cfg.ManifestRefreshTimeout = def.ManifestRefreshTimeout
	}
	if cfg.ManifestBackoffBase <= 0 {
		cfg.ManifestBackoffBase = def.ManifestBackoffBase
	}
	if cfg.ManifestBackoffMax <= 0 {
		cfg.ManifestBackoffMax = def.ManifestBackoffMax
	}
	if cfg.ManifestBackoffMax < cfg.ManifestBackoffBase {
		cfg.ManifestBackoffMax = cfg.ManifestBackoffBase
	}
	if cfg.ManifestBackoffFactor < 1 {
		cfg.ManifestBackoffFactor = def.ManifestBackoffFactor
	}
	if cfg.ManifestBackoffJitterRatio < 0 {
		cfg.ManifestBackoffJitterRatio = 0
	} else if cfg.ManifestBackoffJitterRatio > 1 {
		cfg.ManifestBackoffJitterRatio = 1
	}
	if cfg.MinPeers < 0 {
		cfg.MinPeers = 0
	}
	return cfg
}

func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	n.transitionStateLocked(StateConnecting)
	n.status.LastSync = time.Now()
	n.mu.Unlock()

	if n.cfg.Transport == TransportGoWaku {
		backend := newGoWakuBackend()
		if backend == nil {
			n.setDisconnected()
			return errors.New("go-waku backend is not available in this build")
		}
		if err := backend.Start(ctx, n.cfg); err != nil {
			n.setDisconnected()
			return err
		}
		peerCount := backend.PeerCount()
		if n.cfg.FailoverV1 {
			var err error
			peerCount, err = waitForStartupPeerCount(ctx, backend, n.cfg)
			if err != nil {
				n.setDisconnected()
				return err
			}
		}
		n.mu.Lock()
		n.gw = backend
		n.transitionStateLocked(startupStateFromPeerCount(peerCount, n.cfg))
		n.status.PeerCount = peerCount
		n.status.LastSync = time.Now()
		n.mu.Unlock()
		n.startRuntimeMonitor()
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(50 * time.Millisecond):
	}

	n.mu.Lock()
	n.transitionStateLocked(StateConnected)
	n.status.PeerCount = estimatedPeers(n.cfg)
	n.status.LastSync = time.Now()
	n.mu.Unlock()
	return nil
}

func (n *Node) Stop(_ context.Context) error {
	n.stopRuntimeMonitor()

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.gw != nil {
		n.gw.Stop()
		n.gw = nil
	}
	if n.selfID != "" {
		globalBus.unsubscribe(n.selfID)
	}
	n.transitionStateLocked(StateDisconnected)
	n.status.PeerCount = 0
	n.status.LastSync = time.Now()
	return nil
}

func (n *Node) Status() Status {
	n.mu.RLock()
	defer n.mu.RUnlock()
	s := n.status
	if n.gw != nil {
		s.PeerCount = n.gw.PeerCount()
	}
	if s.BootstrapSource == "" {
		s.BootstrapSource = n.cfg.BootstrapSource
	}
	s.BootstrapManifestVersion = n.cfg.BootstrapManifestVersion
	s.BootstrapManifestKeyID = n.cfg.BootstrapManifestKeyID
	return s
}

func (n *Node) SetIdentity(identityID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.selfID = identityID
	if n.gw != nil {
		n.gw.SetIdentity(identityID)
	}
}

func (n *Node) ApplyBootstrapConfig(cfg Config) {
	cfg = normalizeConfig(cfg)

	n.mu.Lock()
	n.cfg.BootstrapNodes = append([]string(nil), cfg.BootstrapNodes...)
	n.cfg.MinPeers = cfg.MinPeers
	n.cfg.ReconnectInterval = cfg.ReconnectInterval
	n.cfg.ReconnectBackoffMax = cfg.ReconnectBackoffMax
	n.cfg.BootstrapSource = cfg.BootstrapSource
	n.cfg.BootstrapManifestVersion = cfg.BootstrapManifestVersion
	n.cfg.BootstrapManifestKeyID = cfg.BootstrapManifestKeyID
	n.status.BootstrapSource = cfg.BootstrapSource
	n.status.BootstrapManifestVersion = cfg.BootstrapManifestVersion
	n.status.BootstrapManifestKeyID = cfg.BootstrapManifestKeyID
	gw := n.gw
	nodeCfg := n.cfg
	n.mu.Unlock()

	if gw != nil {
		gw.ApplyConfig(nodeCfg)
	}
}

func (n *Node) SubscribePrivate(handler func(PrivateMessage)) error {
	n.mu.Lock()
	n.handler = handler
	state := n.status.State
	selfID := n.selfID
	gw := n.gw
	n.mu.Unlock()

	if state != StateConnected && state != StateDegraded {
		return errors.New("waku not connected")
	}
	if selfID == "" {
		return errors.New("identity is not set")
	}
	if gw != nil {
		return gw.SubscribePrivate(handler)
	}
	globalBus.subscribe(selfID, handler)
	return nil
}

func (n *Node) PublishPrivate(ctx context.Context, msg PrivateMessage) error {
	n.mu.RLock()
	state := n.status.State
	gw := n.gw
	n.mu.RUnlock()
	if state != StateConnected && state != StateDegraded {
		return errors.New("waku not connected")
	}
	if msg.Recipient == "" {
		return errors.New("recipient is required")
	}
	if gw != nil {
		return gw.PublishPrivate(ctx, msg)
	}
	globalBus.publish(msg)
	return nil
}

func (n *Node) ListenAddresses() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.gw == nil {
		return nil
	}
	return append([]string(nil), n.gw.ListenAddresses()...)
}

func (n *Node) FetchPrivateSince(ctx context.Context, recipient string, since time.Time, limit int) ([]PrivateMessage, error) {
	n.mu.RLock()
	state := n.status.State
	gw := n.gw
	n.mu.RUnlock()
	if state != StateConnected && state != StateDegraded {
		return nil, errors.New("waku not connected")
	}
	if recipient == "" {
		return nil, errors.New("recipient is required")
	}
	if gw == nil {
		// Mock transport delivers offline messages via in-memory mailbox on subscription.
		return nil, nil
	}
	return gw.FetchPrivateSince(ctx, recipient, since, limit)
}

func (n *Node) setDisconnected() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.transitionStateLocked(StateDisconnected)
	n.status.PeerCount = 0
	n.status.LastSync = time.Now()
}

func (n *Node) startRuntimeMonitor() {
	n.mu.Lock()
	if n.monitorCancel != nil {
		n.monitorCancel()
		n.monitorCancel = nil
	}
	monitorCtx, cancel := context.WithCancel(context.Background())
	n.monitorCancel = cancel
	n.monitorWG.Add(1)
	n.mu.Unlock()

	go func() {
		defer n.monitorWG.Done()
		ticker := time.NewTicker(runtimeStatusPollInterval)
		defer ticker.Stop()

		// Update once immediately to avoid waiting one interval after startup.
		n.refreshRuntimeStatus()

		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-ticker.C:
				n.refreshRuntimeStatus()
			}
		}
	}()
}

func (n *Node) stopRuntimeMonitor() {
	n.mu.Lock()
	cancel := n.monitorCancel
	n.monitorCancel = nil
	n.mu.Unlock()
	if cancel != nil {
		cancel()
		n.monitorWG.Wait()
	}
}

func (n *Node) refreshRuntimeStatus() {
	n.mu.RLock()
	gw := n.gw
	n.mu.RUnlock()
	if gw == nil {
		return
	}
	peerCount := gw.PeerCount()
	nextState := StateConnected
	if peerCount <= 0 {
		nextState = StateDegraded
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.status.State == StateDisconnected {
		return
	}
	if n.status.State != nextState || n.status.PeerCount != peerCount {
		n.transitionStateLocked(nextState)
		n.status.PeerCount = peerCount
		n.status.LastSync = time.Now()
	}
}

func (n *Node) NetworkMetrics() map[string]int {
	n.mu.RLock()
	transitions := n.stateTransitions
	gw := n.gw
	n.mu.RUnlock()
	out := map[string]int{
		"network_state_transitions": transitions,
	}
	if gw != nil {
		for k, v := range gw.NetworkMetrics() {
			out[k] = v
		}
	}
	return out
}

func (n *Node) transitionStateLocked(next string) {
	if next == "" {
		return
	}
	if n.status.State != next {
		n.stateTransitions++
		n.status.State = next
	}
}

func estimatedPeers(cfg Config) int {
	if len(cfg.BootstrapNodes) == 0 {
		return 1
	}
	if len(cfg.BootstrapNodes) > 12 {
		return 12
	}
	return len(cfg.BootstrapNodes)
}

func waitForStartupPeerCount(ctx context.Context, backend goWakuBackend, cfg Config) (int, error) {
	target := startupPeerTarget(cfg)
	peerCount := backend.PeerCount()
	if peerCount >= target {
		return peerCount, nil
	}

	timeout := startupHandshakeTimeout(cfg)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return backend.PeerCount(), ctx.Err()
		case <-timer.C:
			return backend.PeerCount(), nil
		case <-ticker.C:
			peerCount = backend.PeerCount()
			if peerCount >= target {
				return peerCount, nil
			}
		}
	}
}

func startupStateFromPeerCount(peerCount int, cfg Config) string {
	if peerCount >= startupPeerTarget(cfg) {
		return StateConnected
	}
	return StateDegraded
}

func startupPeerTarget(cfg Config) int {
	target := cfg.MinPeers
	if target <= 0 {
		target = 1
	}
	if len(cfg.BootstrapNodes) > 0 && target > len(cfg.BootstrapNodes) {
		target = len(cfg.BootstrapNodes)
	}
	if target < 1 {
		target = 1
	}
	return target
}

func startupHandshakeTimeout(cfg Config) time.Duration {
	base := cfg.ReconnectInterval
	if base <= 0 {
		base = time.Second
	}
	timeout := base * 5
	if timeout < 2*time.Second {
		timeout = 2 * time.Second
	}
	if cfg.ReconnectBackoffMax > 0 && timeout > cfg.ReconnectBackoffMax {
		timeout = cfg.ReconnectBackoffMax
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return timeout
}
