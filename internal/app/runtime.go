package app

import (
	"aim-chat/go-backend/pkg/models"
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"sync"
	"time"
)

type OpMetric struct {
	Count   int
	Errors  int
	TotalNs int64
	MaxNs   int64
	LastNs  int64
}

type NotificationEvent struct {
	Seq       int64
	Method    string
	Payload   any
	Timestamp time.Time
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

type NotificationHub struct {
	mu      sync.Mutex
	nextSeq int64
	limit   int
	history []NotificationEvent
	subs    map[int]chan NotificationEvent
	nextSub int
}

func NewNotificationHub(limit int) *NotificationHub {
	if limit < 1 {
		limit = 1
	}
	return &NotificationHub{
		limit: limit,
		subs:  make(map[int]chan NotificationEvent),
	}
}

func (h *NotificationHub) Publish(method string, payload any) NotificationEvent {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextSeq++
	event := NotificationEvent{
		Seq:       h.nextSeq,
		Method:    method,
		Payload:   payload,
		Timestamp: nowUTC(),
	}
	h.history = append(h.history, event)
	if len(h.history) > h.limit {
		h.history = append([]NotificationEvent(nil), h.history[len(h.history)-h.limit:]...)
	}

	for id, ch := range h.subs {
		select {
		case ch <- event:
		default:
			close(ch)
			delete(h.subs, id)
		}
	}

	return event
}

func (h *NotificationHub) Subscribe(fromSeq int64) ([]NotificationEvent, <-chan NotificationEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	replay := make([]NotificationEvent, 0)
	for _, event := range h.history {
		if event.Seq > fromSeq {
			replay = append(replay, event)
		}
	}

	id := h.nextSub
	h.nextSub++
	ch := make(chan NotificationEvent, 128)
	h.subs[id] = ch

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if sub, ok := h.subs[id]; ok {
			close(sub)
			delete(h.subs, id)
		}
	}
	return replay, ch, cancel
}

func (h *NotificationHub) BacklogSize() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.history)
}

type ServiceRuntime struct {
	Mu              sync.RWMutex
	Networking      bool
	NetworkCtx      context.Context
	NetworkCancel   context.CancelFunc
	RetryCancel     context.CancelFunc
	RetryWG         sync.WaitGroup
	NetworkStateSet bool
	LastNetwork     models.NetworkStatus
}

func NewServiceRuntime() *ServiceRuntime {
	return &ServiceRuntime{}
}

func (r *ServiceRuntime) IsNetworking() bool {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	return r.Networking
}

func (r *ServiceRuntime) TryActivate(networkCtx context.Context, networkCancel, retryCancel context.CancelFunc) bool {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if r.Networking {
		return false
	}
	r.NetworkCtx = networkCtx
	r.NetworkCancel = networkCancel
	r.RetryCancel = retryCancel
	r.RetryWG.Add(1)
	r.Networking = true
	return true
}

func (r *ServiceRuntime) RetryLoopDone() {
	r.RetryWG.Done()
}

func (r *ServiceRuntime) Deactivate() (retryCancel, networkCancel context.CancelFunc, wasRunning bool) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if !r.Networking {
		return nil, nil, false
	}
	retryCancel = r.RetryCancel
	networkCancel = r.NetworkCancel
	r.RetryCancel = nil
	r.NetworkCancel = nil
	r.NetworkCtx = nil
	r.Networking = false
	return retryCancel, networkCancel, true
}

func (r *ServiceRuntime) WaitRetryLoop() {
	r.RetryWG.Wait()
}

func (r *ServiceRuntime) CurrentNetworkContext() (context.Context, bool) {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	if !r.Networking || r.NetworkCtx == nil {
		return nil, false
	}
	return r.NetworkCtx, true
}

func (r *ServiceRuntime) UpdateLastNetworkStatus(current models.NetworkStatus, force bool) bool {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	changed := !r.NetworkStateSet ||
		r.LastNetwork.Status != current.Status ||
		r.LastNetwork.PeerCount != current.PeerCount
	if force || changed {
		r.LastNetwork = current
		r.NetworkStateSet = true
	}
	return force || changed
}

func DefaultLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

func GeneratePrefixedID(prefix string) (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(buf), nil
}

type ServiceMetricsState struct {
	mu            sync.RWMutex
	errorCounters map[string]int
	opMetrics     map[string]*OpMetric
	retryAttempts int
	lastUpdatedAt time.Time
}

func NewServiceMetricsState() *ServiceMetricsState {
	return &ServiceMetricsState{
		errorCounters: map[string]int{
			"api":     0,
			"network": 0,
			"crypto":  0,
			"storage": 0,
		},
		opMetrics: map[string]*OpMetric{},
	}
}

func (m *ServiceMetricsState) Snapshot() (map[string]int, map[string]models.OperationMetric, int, time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counters := make(map[string]int, len(m.errorCounters))
	for k, v := range m.errorCounters {
		counters[k] = v
	}
	opStats := make(map[string]models.OperationMetric, len(m.opMetrics))
	for name, metric := range m.opMetrics {
		avg := int64(0)
		if metric.Count > 0 {
			avg = metric.TotalNs / int64(metric.Count) / int64(time.Millisecond)
		}
		opStats[name] = models.OperationMetric{
			Count:         metric.Count,
			Errors:        metric.Errors,
			AvgLatencyMs:  avg,
			MaxLatencyMs:  metric.MaxNs / int64(time.Millisecond),
			LastLatencyMs: metric.LastNs / int64(time.Millisecond),
		}
	}
	return counters, opStats, m.retryAttempts, m.lastUpdatedAt
}

func (m *ServiceMetricsState) RecordError(category string) {
	m.mu.Lock()
	m.errorCounters[category] = m.errorCounters[category] + 1
	m.lastUpdatedAt = time.Now().UTC()
	m.mu.Unlock()
}

func (m *ServiceMetricsState) RecordRetryAttempt() {
	m.mu.Lock()
	m.retryAttempts++
	m.lastUpdatedAt = time.Now().UTC()
	m.mu.Unlock()
}

func (m *ServiceMetricsState) RecordOp(operation string, started time.Time) {
	latency := time.Since(started).Nanoseconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	metric, ok := m.opMetrics[operation]
	if !ok {
		metric = &OpMetric{}
		m.opMetrics[operation] = metric
	}
	metric.Count++
	metric.TotalNs += latency
	metric.LastNs = latency
	if latency > metric.MaxNs {
		metric.MaxNs = latency
	}
	m.lastUpdatedAt = time.Now().UTC()
}

func (m *ServiceMetricsState) RecordOpError(operation string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	metric, ok := m.opMetrics[operation]
	if !ok {
		metric = &OpMetric{}
		m.opMetrics[operation] = metric
	}
	metric.Errors++
	m.lastUpdatedAt = time.Now().UTC()
}
