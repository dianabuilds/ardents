package rpc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRequiresRPCToken_DefaultIsTrueInProdLikeEnv(t *testing.T) {
	t.Setenv("AIM_REQUIRE_RPC_TOKEN", "")
	t.Setenv("AIM_ENV", "production")
	if !requiresRPCToken() {
		t.Fatal("expected rpc token to be required in production-like env")
	}
}

func TestRequiresRPCToken_DefaultIsFalseInNonProdEnv(t *testing.T) {
	t.Setenv("AIM_REQUIRE_RPC_TOKEN", "")
	t.Setenv("AIM_ENV", "development")
	if requiresRPCToken() {
		t.Fatal("expected rpc token to be optional in non-prod env")
	}
}

func TestRequiresRPCToken_FalseOverrideIsIgnoredInProdLikeEnv(t *testing.T) {
	t.Setenv("AIM_REQUIRE_RPC_TOKEN", "false")
	t.Setenv("AIM_ENV", "production")
	if !requiresRPCToken() {
		t.Fatal("expected fail-closed token requirement in production-like env")
	}
}

func TestRequiresRPCToken_FalseOverrideAllowedInNonProdEnv(t *testing.T) {
	t.Setenv("AIM_REQUIRE_RPC_TOKEN", "false")
	t.Setenv("AIM_ENV", "test")
	if requiresRPCToken() {
		t.Fatal("expected false override to work in non-prod env")
	}
}

func TestExtractRPCToken_PrefersCustomHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/rpc", nil)
	req.Header.Set("X-AIM-RPC-Token", "header-token")
	req.Header.Set("Authorization", "Bearer bearer-token")

	s := &Server{}
	got := s.extractRPCToken(req)
	if got != "header-token" {
		t.Fatalf("expected header token, got %q", got)
	}
}

func TestExtractRPCToken_UsesBearerHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/rpc", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")

	s := &Server{}
	got := s.extractRPCToken(req)
	if got != "bearer-token" {
		t.Fatalf("expected bearer token, got %q", got)
	}
}

func TestIsAllowedOrigin_LocalhostOnly(t *testing.T) {
	t.Setenv("AIM_ALLOW_NULL_ORIGIN", "false")
	cases := []struct {
		origin string
		want   bool
	}{
		{"http://localhost:3000", true},
		{"https://127.0.0.1:8787", true},
		{"http://[::1]:8787", true},
		{"https://example.com", false},
		{"not-a-url", false},
	}
	for _, tc := range cases {
		if got := isAllowedOrigin(tc.origin); got != tc.want {
			t.Fatalf("origin %q: got %v, want %v", tc.origin, got, tc.want)
		}
	}
}

func TestResolveRPCToken_AutoRotatesAndPersistsToFile(t *testing.T) {
	t.Setenv("AIM_RPC_TOKEN", "auto")
	tokenFile := filepath.Join(t.TempDir(), "runtime", "rpc.token")
	t.Setenv("AIM_RPC_TOKEN_FILE", tokenFile)

	token, err := resolveRPCToken()
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token == "" || token == "auto" {
		t.Fatalf("expected generated token, got %q", token)
	}

	raw, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if string(raw) != token {
		t.Fatalf("unexpected token file content")
	}
}

func TestResolveRPCToken_RotateOnStartOverridesStaticToken(t *testing.T) {
	t.Setenv("AIM_RPC_TOKEN", "static-token")
	t.Setenv("AIM_RPC_TOKEN_ROTATE_ON_START", "true")

	token, err := resolveRPCToken()
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token == "" || token == "static-token" {
		t.Fatalf("expected rotated token, got %q", token)
	}
}

func TestApplyCORS_SetsSecurityHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/rpc", nil)
	rr := httptest.NewRecorder()
	s := &Server{rpcToken: "token", requireRPC: true}

	if ok := s.applyCORS(rr, req); !ok {
		t.Fatal("expected CORS apply to succeed")
	}

	headers := rr.Result().Header
	if got := headers.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("unexpected X-Content-Type-Options: %q", got)
	}
	if got := headers.Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("unexpected Referrer-Policy: %q", got)
	}
	if got := headers.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("unexpected X-Frame-Options: %q", got)
	}
	if got := headers.Get("Permissions-Policy"); got == "" {
		t.Fatal("expected Permissions-Policy header")
	}
}

func TestApplyCORS_RejectsOriginWhenAuthDisabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()
	s := &Server{rpcToken: "", requireRPC: false}

	if ok := s.applyCORS(rr, req); ok {
		t.Fatal("expected CORS apply to fail when auth is disabled and origin is set")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden status, got %d", rr.Code)
	}
}
