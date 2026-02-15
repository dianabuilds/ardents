package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
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
	maxMessageListLimit  = 1000
	maxMessageListOffset = 1_000_000
)

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if !s.applyCORS(w, r) {
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.authorizeRPC(w, r) {
		return
	}
	if s.service == nil {
		writeRPC(w, rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32099, Message: "service is not initialized"},
		})
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
	reqID := fmt.Sprintf("rpc_%d", time.Now().UnixNano())
	started := time.Now()
	slog.Default().Info("rpc request", "request_id", reqID, "method", req.Method, "rpc_id", string(req.ID))

	result, rpcErr := s.dispatchRPC(req.Method, req.Params)
	if rpcErr != nil {
		slog.Default().Error("rpc failed", "request_id", reqID, "method", req.Method, "rpc_code", rpcErr.Code, "latency_ms", time.Since(started).Milliseconds())
	} else {
		slog.Default().Info("rpc response", "request_id", reqID, "method", req.Method, "latency_ms", time.Since(started).Milliseconds())
	}
	resp := rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	}
	writeRPC(w, resp)
}

func (s *Server) dispatchRPC(method string, rawParams json.RawMessage) (any, *rpcError) {
	if method == "health_check" {
		return map[string]string{"status": "ok"}, nil
	}
	if result, rpcErr, ok := s.dispatchIdentityRPC(method, rawParams); ok {
		return result, rpcErr
	}
	if result, rpcErr, ok := s.dispatchNetworkRPC(method); ok {
		return result, rpcErr
	}
	if result, rpcErr, ok := s.dispatchSessionMessageRPC(method, rawParams); ok {
		return result, rpcErr
	}
	if result, rpcErr, ok := s.dispatchContactFileRPC(method, rawParams); ok {
		return result, rpcErr
	}
	if result, rpcErr, ok := s.dispatchDeviceRPC(method, rawParams); ok {
		return result, rpcErr
	}
	return nil, &rpcError{Code: -32601, Message: "method not found"}
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
