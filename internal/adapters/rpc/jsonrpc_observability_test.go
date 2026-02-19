package rpc

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRPCCorrelationIDFromHeader(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"rpc.version","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AIM-Request-ID", "ui.42.health_check")
	rec := httptest.NewRecorder()
	s.HandleRPC(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	got := rec.Header().Get("X-AIM-Request-ID")
	if got != "ui.42.health_check" {
		t.Fatalf("unexpected response request id: got=%q", got)
	}
}

func TestRPCCorrelationIDFallbackFromRPCID(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":"abc-1","method":"rpc.version","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.HandleRPC(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	got := rec.Header().Get("X-AIM-Request-ID")
	if got != "rpc._abc-1_" {
		t.Fatalf("unexpected fallback response request id: got=%q", got)
	}
}
