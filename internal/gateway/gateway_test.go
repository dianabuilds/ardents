package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayAuthReject(t *testing.T) {
	h := NewHandler(Service{Token: "secret", Status: func() any { return map[string]any{"status": "ok"} }})
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestGatewaySendOK(t *testing.T) {
	h := NewHandler(Service{
		Token: "secret",
		Send: func(ctx context.Context, addr, to, text string) (AckResult, error) {
			return AckResult{Status: "OK", ErrorCode: ""}, nil
		},
		Status: func() any { return map[string]any{"status": "ok"} },
	})
	body := map[string]any{"to": "peer_x", "text": "hi", "addr": "quic://127.0.0.1:1"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "OK" {
		t.Fatalf("unexpected status: %v", resp["status"])
	}
}

func TestGatewayResolveNotFound(t *testing.T) {
	h := NewHandler(Service{
		Token: "secret",
		Resolve: func(alias string) (ResolveResult, error) {
			return ResolveResult{Found: false}, nil
		},
		Status: func() any { return map[string]any{"status": "ok"} },
	})
	body := map[string]any{"alias": "missing"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/resolve", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
