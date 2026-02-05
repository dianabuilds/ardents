package metrics

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Registry struct {
	mu sync.Mutex

	netInboundConns  uint64
	netOutboundConns uint64
	peersConnected   uint64
	msgReceived      map[string]uint64
	msgRejected      map[string]uint64
	powRequired      uint64
	powInvalid       uint64
	clockInvalid     uint64
	taskRequested    map[string]uint64
	taskResult       map[string]uint64
	taskFail         map[string]uint64
	taskTimeout      uint64
	ipcErrors        map[string]uint64
	ipcTimeout       uint64
	ackLatencyCount  uint64
	ackLatencySumMs  uint64
	ackLatencyBucket []uint64
}

func New() *Registry {
	return &Registry{
		msgReceived:      make(map[string]uint64),
		msgRejected:      make(map[string]uint64),
		taskRequested:    make(map[string]uint64),
		taskResult:       make(map[string]uint64),
		taskFail:         make(map[string]uint64),
		ipcErrors:        make(map[string]uint64),
		ackLatencyBucket: make([]uint64, len(ackLatencyBucketBounds)),
	}
}

func (r *Registry) IncMsgReceived(typ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgReceived[typ]++
}

func (r *Registry) IncMsgRejected(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgRejected[reason]++
}

func (r *Registry) IncPowRequired() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.powRequired++
}

func (r *Registry) IncPowInvalid() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.powInvalid++
}

func (r *Registry) IncClockInvalid() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clockInvalid++
}

func (r *Registry) IncNetInbound() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.netInboundConns++
}

func (r *Registry) DecNetInbound() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.netInboundConns > 0 {
		r.netInboundConns--
	}
}

func (r *Registry) IncNetOutbound() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.netOutboundConns++
}

func (r *Registry) DecNetOutbound() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.netOutboundConns > 0 {
		r.netOutboundConns--
	}
}

func (r *Registry) SetPeersConnected(count uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peersConnected = count
}

func (r *Registry) IncTaskRequested(jobType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if jobType == "" {
		jobType = "unknown"
	}
	r.taskRequested[jobType]++
}

func (r *Registry) IncTaskResult(jobType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if jobType == "" {
		jobType = "unknown"
	}
	r.taskResult[jobType]++
}

func (r *Registry) IncTaskFail(code string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if code == "" {
		code = "ERR_TASK_FAILED"
	}
	r.taskFail[code]++
}

func (r *Registry) IncTaskTimeout() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.taskTimeout++
}

func (r *Registry) IncIPCErr(code string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if code == "" {
		code = "ERR_IPC"
	}
	r.ipcErrors[code]++
}

func (r *Registry) IncIPCTimeout() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ipcTimeout++
}

func (r *Registry) ObserveAckLatency(ms uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ackLatencyCount++
	r.ackLatencySumMs += ms
	for i, b := range ackLatencyBucketBounds {
		if ms <= b {
			r.ackLatencyBucket[i]++
			return
		}
	}
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		r.mu.Lock()
		defer r.mu.Unlock()
		var buf bytes.Buffer
		if _, err := fmt.Fprintf(&buf, "net_inbound_conns %d\n", r.netInboundConns); err != nil {
			return
		}
		if _, err := fmt.Fprintf(&buf, "net_outbound_conns %d\n", r.netOutboundConns); err != nil {
			return
		}
		if _, err := fmt.Fprintf(&buf, "peers_connected %d\n", r.peersConnected); err != nil {
			return
		}
		for k, v := range r.msgReceived {
			if _, err := fmt.Fprintf(&buf, "msg_received_total{type=\"%s\"} %d\n", k, v); err != nil {
				return
			}
		}
		for k, v := range r.msgRejected {
			if _, err := fmt.Fprintf(&buf, "msg_rejected_total{reason=\"%s\"} %d\n", k, v); err != nil {
				return
			}
		}
		if _, err := fmt.Fprintf(&buf, "pow_required_total %d\n", r.powRequired); err != nil {
			return
		}
		if _, err := fmt.Fprintf(&buf, "pow_invalid_total %d\n", r.powInvalid); err != nil {
			return
		}
		if _, err := fmt.Fprintf(&buf, "clock_invalid_total %d\n", r.clockInvalid); err != nil {
			return
		}
		for k, v := range r.taskRequested {
			if _, err := fmt.Fprintf(&buf, "task_request_total{job_type=\"%s\"} %d\n", k, v); err != nil {
				return
			}
		}
		for k, v := range r.taskResult {
			if _, err := fmt.Fprintf(&buf, "task_result_total{job_type=\"%s\"} %d\n", k, v); err != nil {
				return
			}
		}
		for k, v := range r.taskFail {
			if _, err := fmt.Fprintf(&buf, "task_fail_total{code=\"%s\"} %d\n", k, v); err != nil {
				return
			}
		}
		if _, err := fmt.Fprintf(&buf, "task_timeout_total %d\n", r.taskTimeout); err != nil {
			return
		}
		for k, v := range r.ipcErrors {
			if _, err := fmt.Fprintf(&buf, "ipc_errors_total{code=\"%s\"} %d\n", k, v); err != nil {
				return
			}
		}
		if _, err := fmt.Fprintf(&buf, "ipc_timeout_total %d\n", r.ipcTimeout); err != nil {
			return
		}
		var cum uint64
		for i, b := range ackLatencyBucketBounds {
			cum += r.ackLatencyBucket[i]
			if _, err := fmt.Fprintf(&buf, "ack_latency_ms_bucket{le=\"%d\"} %d\n", b, cum); err != nil {
				return
			}
		}
		if _, err := fmt.Fprintf(&buf, "ack_latency_ms_bucket{le=\"+Inf\"} %d\n", r.ackLatencyCount); err != nil {
			return
		}
		if _, err := fmt.Fprintf(&buf, "ack_latency_ms_sum %d\n", r.ackLatencySumMs); err != nil {
			return
		}
		if _, err := fmt.Fprintf(&buf, "ack_latency_ms_count %d\n", r.ackLatencyCount); err != nil {
			return
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			return
		}
	})
}

var ackLatencyBucketBounds = []uint64{50, 100, 250, 500, 1000, 2000, 5000}

type Server struct {
	srv *http.Server
}

func Start(addr string, reg *Registry) *Server {
	s := &http.Server{
		Addr:              addr,
		Handler:           reg.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return
		}
	}()
	return &Server{srv: s}
}

func (s *Server) Stop() {
	if s == nil || s.srv == nil {
		return
	}
	if err := s.srv.Close(); err != nil {
		return
	}
}
