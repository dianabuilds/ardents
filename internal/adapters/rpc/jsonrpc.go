package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	grouprpc "aim-chat/go-backend/internal/domains/group/adapters/rpc"
	identityrpc "aim-chat/go-backend/internal/domains/identity/adapters/rpc"
	inboxrpc "aim-chat/go-backend/internal/domains/inbox/adapters/rpc"
	messagingrpc "aim-chat/go-backend/internal/domains/messaging/adapters/rpc"
	privacyrpc "aim-chat/go-backend/internal/domains/privacy/adapters/rpc"
	"aim-chat/go-backend/internal/domains/rpckit"
)

type rpcRequest struct {
	JSONRPC    string          `json:"jsonrpc"`
	ID         json.RawMessage `json:"id"`
	Method     string          `json:"method"`
	Params     json.RawMessage `json:"params"`
	APIVersion *int            `json:"api_version,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const maxRPCBodyBytes int64 = 1 << 20 // 1 MiB
const (
	rpcRequestIDHeader = "X-AIM-Request-ID"
)

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if !s.applyCORS(w, r) {
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.rpcLimiter.allow(rpcRateLimitKey(r, s.extractRPCToken(r)), time.Now()) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	if !s.authorizeRPC(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRPCBodyBytes)
	var req rpcRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		writeRPC(w, rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeRPCInvalidRequest(w, req.ID)
		return
	}

	if req.JSONRPC != "2.0" || req.Method == "" {
		writeRPCInvalidRequest(w, req.ID)
		return
	}
	if strings.HasPrefix(req.Method, "node.") && !isLoopbackRequest(r) {
		writeRPC(w, rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32084, Message: "node methods are available only from loopback client"},
		})
		return
	}
	if versionErr := validateRPCAPIVersion(req.APIVersion); versionErr != nil {
		writeRPC(w, rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   versionErr,
		})
		return
	}
	idempotencyKey := rpcIdempotencyKey(r.Header.Get(rpcIdempotencyHeader), s.extractRPCToken(r))
	requestHash := ""
	if idempotencyKey != "" {
		requestHash = rpcRequestHash(req)
		s.idempotencyMu.Lock()
		cached, found, conflict := s.idempotency.get(idempotencyKey, requestHash, time.Now().UTC())
		s.idempotencyMu.Unlock()
		if conflict {
			writeRPC(w, rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32082, Message: "idempotency key reuse with different request payload"},
			})
			return
		}
		if found {
			cached.ID = req.ID
			writeRPC(w, cached)
			return
		}
	}
	if s.service == nil && req.Method != "rpc.version" && req.Method != "rpc.capabilities" {
		resp := rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32099, Message: "service is not initialized"},
		}
		if idempotencyKey != "" {
			s.idempotencyMu.Lock()
			s.idempotency.set(idempotencyKey, requestHash, resp, time.Now().UTC())
			s.idempotencyMu.Unlock()
		}
		writeRPC(w, resp)
		return
	}
	reqID := resolveRPCRequestID(r, req.ID)
	w.Header().Set(rpcRequestIDHeader, reqID)
	started := time.Now()
	slog.Default().Info("rpc request", "correlation_id", reqID, "request_id", reqID, "method", req.Method, "rpc_id", string(req.ID))

	result, rpcErr := s.dispatchRPC(req.Method, req.Params)
	if rpcErr != nil {
		slog.Default().Error("rpc failed", "correlation_id", reqID, "request_id", reqID, "method", req.Method, "rpc_code", rpcErr.Code, "latency_ms", time.Since(started).Milliseconds())
	} else {
		slog.Default().Info("rpc response", "correlation_id", reqID, "request_id", reqID, "method", req.Method, "latency_ms", time.Since(started).Milliseconds())
	}
	resp := rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	}
	if idempotencyKey != "" {
		s.idempotencyMu.Lock()
		s.idempotency.set(idempotencyKey, requestHash, resp, time.Now().UTC())
		s.idempotencyMu.Unlock()
	}
	writeRPC(w, resp)
}

func (s *Server) dispatchRPC(method string, rawParams json.RawMessage) (any, *rpcError) {
	if result, rpcErr, ok := s.dispatchCoreRPC(method); ok {
		return result, rpcErr
	}
	if result, rpcErr, ok := identityrpc.Dispatch(s.service, method, rawParams); ok {
		return result, mapKitError(rpcErr)
	}
	if result, rpcErr, ok := privacyrpc.Dispatch(s.service, method, rawParams); ok {
		return result, mapKitError(rpcErr)
	}
	if result, rpcErr, ok := inboxrpc.Dispatch(s.service, method, rawParams); ok {
		return result, mapKitError(rpcErr)
	}
	if result, rpcErr, ok := messagingrpc.Dispatch(s.service, method, rawParams); ok {
		return result, mapKitError(rpcErr)
	}
	if (strings.HasPrefix(method, "group.") || strings.HasPrefix(method, "channel.")) && !s.groupsEnabled {
		return nil, &rpcError{Code: -32199, Message: "groups feature is disabled"}
	}
	if result, rpcErr, ok := grouprpc.Dispatch(s.service, method, rawParams); ok {
		return result, mapKitError(rpcErr)
	}
	if result, rpcErr, ok := s.dispatchNetworkRPC(method); ok {
		return result, rpcErr
	}
	return nil, &rpcError{Code: -32601, Message: "method not found"}
}

func (s *Server) dispatchCoreRPC(method string) (any, *rpcError, bool) {
	switch method {
	case "rpc.version":
		return rpcVersionInfo(), nil, true
	case "rpc.capabilities":
		return rpcCapabilitiesInfo(), nil, true
	case "health_check":
		return map[string]string{"status": "ok"}, nil, true
	default:
		return nil, nil, false
	}
}

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeRPCInvalidRequest(w http.ResponseWriter, id json.RawMessage) {
	writeRPC(w, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: -32600, Message: "invalid request"},
	})
}

func resolveRPCRequestID(r *http.Request, rpcID json.RawMessage) string {
	headerID := sanitizeRPCRequestID(r.Header.Get(rpcRequestIDHeader))
	if headerID != "" {
		return headerID
	}
	trimmedRPCID := strings.TrimSpace(string(rpcID))
	if trimmedRPCID != "" {
		if fallback := sanitizeRPCRequestID(trimmedRPCID); fallback != "" {
			return "rpc." + fallback
		}
	}
	return fmt.Sprintf("rpc.%d", time.Now().UnixNano())
}

func sanitizeRPCRequestID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'):
			b.WriteByte(c)
		case c == '.', c == '_', c == '-', c == ':':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 128 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func isLoopbackRequest(r *http.Request) bool {
	remote := strings.TrimSpace(r.RemoteAddr)
	if remote == "" {
		return false
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func mapKitError(err *rpckit.Error) *rpcError {
	if err == nil {
		return nil
	}
	return &rpcError{
		Code:    err.Code,
		Message: err.Message,
	}
}
