package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRPCHealthzContract(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health payload: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", body["status"])
	}
}

func TestRPCRejectsUnauthorizedRequest(t *testing.T) {
	t.Setenv("AIM_REQUIRE_RPC_TOKEN", "true")
	t.Setenv("AIM_RPC_TOKEN", "secret-token")

	s := NewServerWithService(DefaultRPCAddr, nil)
	if s.initErr != nil {
		t.Fatalf("unexpected init error: %v", s.initErr)
	}

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"health_check","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.HandleRPC(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRPCAuthAcceptedButServiceMissing(t *testing.T) {
	t.Setenv("AIM_REQUIRE_RPC_TOKEN", "true")
	t.Setenv("AIM_RPC_TOKEN", "secret-token")

	s := NewServerWithService(DefaultRPCAddr, nil)
	if s.initErr != nil {
		t.Fatalf("unexpected init error: %v", s.initErr)
	}

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"health_check","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AIM-RPC-Token", "secret-token")
	rec := httptest.NewRecorder()
	s.HandleRPC(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	var resp rpcResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode rpc response: %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("expected rpc error, got nil")
	}
	if resp.Error.Code != -32099 {
		t.Fatalf("expected rpc code -32099, got %d", resp.Error.Code)
	}
}
