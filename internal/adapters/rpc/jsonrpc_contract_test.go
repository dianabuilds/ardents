package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func rpcCall(t *testing.T, s *Server, body string, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-AIM-RPC-Token", token)
	}
	rec := httptest.NewRecorder()
	s.HandleRPC(rec, req)
	return rec
}

func rpcCallWithRemoteAddr(t *testing.T, s *Server, body string, token string, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(body))
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-AIM-RPC-Token", token)
	}
	rec := httptest.NewRecorder()
	s.HandleRPC(rec, req)
	return rec
}

func decodeRPCResponse(t *testing.T, rec *httptest.ResponseRecorder) rpcResponse {
	t.Helper()
	var resp rpcResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode rpc response: %v", err)
	}
	return resp
}

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

	rec := rpcCall(t, s, `{"jsonrpc":"2.0","id":1,"method":"health_check","params":{}}`, "")

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

	rec := rpcCall(t, s, `{"jsonrpc":"2.0","id":1,"method":"health_check","params":{}}`, "secret-token")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	resp := decodeRPCResponse(t, rec)
	if resp.Error == nil {
		t.Fatalf("expected rpc error, got nil")
	}
	if resp.Error.Code != -32099 {
		t.Fatalf("expected rpc code -32099, got %d", resp.Error.Code)
	}
}

func TestRPCVersionMethodWorksWithoutServiceInitialization(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	rec := rpcCall(t, s, `{"jsonrpc":"2.0","id":1,"method":"rpc.version","params":{}}`, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	resp := decodeRPCResponse(t, rec)
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %#v", resp.Result)
	}
	current, ok := result["current_version"].(float64)
	if !ok || int(current) != rpcAPICurrentVersion {
		t.Fatalf("unexpected current_version: %#v", result["current_version"])
	}
	minSupported, ok := result["min_supported_version"].(float64)
	if !ok || int(minSupported) != rpcAPIMinSupportedVersion {
		t.Fatalf("unexpected min_supported_version: %#v", result["min_supported_version"])
	}
}

func TestRPCCapabilitiesMethodWorksWithoutServiceInitialization(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	rec := rpcCall(t, s, `{"jsonrpc":"2.0","id":1,"method":"rpc.capabilities","params":{}}`, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	resp := decodeRPCResponse(t, rec)
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %#v", resp.Result)
	}
	rawMethods, ok := result["methods"].([]any)
	if !ok {
		t.Fatalf("expected methods array, got %#v", result["methods"])
	}
	foundDiagnostics := false
	for _, method := range rawMethods {
		if name, ok := method.(string); ok && name == "diagnostics.export" {
			foundDiagnostics = true
			break
		}
	}
	if !foundDiagnostics {
		t.Fatal("expected diagnostics.export in rpc capabilities")
	}
}

func TestRPCRejectsUnsupportedFutureAPIVersion(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	rec := rpcCall(t, s, `{"jsonrpc":"2.0","id":1,"method":"health_check","api_version":999,"params":{}}`, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	resp := decodeRPCResponse(t, rec)
	if resp.Error == nil {
		t.Fatalf("expected rpc error, got nil")
	}
	if resp.Error.Code != -32080 {
		t.Fatalf("expected rpc code -32080, got %d", resp.Error.Code)
	}
}

func TestRPCRejectsDeprecatedAPIVersion(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	rec := rpcCall(t, s, `{"jsonrpc":"2.0","id":1,"method":"health_check","api_version":0,"params":{}}`, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	resp := decodeRPCResponse(t, rec)
	if resp.Error == nil {
		t.Fatalf("expected rpc error, got nil")
	}
	if resp.Error.Code != -32081 {
		t.Fatalf("expected rpc code -32081, got %d", resp.Error.Code)
	}
}

func TestRPCNodeMethodsRejectNonLoopbackClient(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	rec := rpcCallWithRemoteAddr(
		t,
		s,
		`{"jsonrpc":"2.0","id":1,"method":"node.getPolicies","params":[]}`,
		"",
		"198.51.100.23:61234",
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	resp := decodeRPCResponse(t, rec)
	if resp.Error == nil {
		t.Fatal("expected rpc error")
	}
	if resp.Error.Code != -32084 {
		t.Fatalf("expected rpc code -32084, got %d", resp.Error.Code)
	}
}

func TestRPCNodeMethodsAllowLoopbackClient(t *testing.T) {
	s := newServerWithService(DefaultRPCAddr, nil, "", false)

	rec := rpcCallWithRemoteAddr(
		t,
		s,
		`{"jsonrpc":"2.0","id":1,"method":"node.getPolicies","params":[]}`,
		"",
		"127.0.0.1:61234",
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	resp := decodeRPCResponse(t, rec)
	if resp.Error == nil {
		t.Fatal("expected rpc error because service is nil")
	}
	if resp.Error.Code != -32099 {
		t.Fatalf("expected rpc code -32099, got %d", resp.Error.Code)
	}
}
