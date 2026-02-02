package metrics

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Registry struct {
	mu sync.Mutex

	netInboundConns  uint64
	netOutboundConns uint64
	msgReceived      map[string]uint64
	msgRejected      map[string]uint64
	powRequired      uint64
	powInvalid       uint64
	ackLatencyMs     []uint64
}

func New() *Registry {
	return &Registry{
		msgReceived:  make(map[string]uint64),
		msgRejected:  make(map[string]uint64),
		ackLatencyMs: make([]uint64, 0),
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

func (r *Registry) ObserveAckLatency(ms uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ackLatencyMs = append(r.ackLatencyMs, ms)
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
		if len(r.ackLatencyMs) > 0 {
			var sum uint64
			for _, v := range r.ackLatencyMs {
				sum += v
			}
			if _, err := fmt.Fprintf(&buf, "ack_latency_ms_avg %d\n", sum/uint64(len(r.ackLatencyMs))); err != nil {
				return
			}
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			return
		}
	})
}

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
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
