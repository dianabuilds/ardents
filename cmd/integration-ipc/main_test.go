package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

func TestParseUpstreamLoopbackOnly(t *testing.T) {
	if _, err := parseUpstream("https://127.0.0.1:8080"); err != nil {
		t.Fatalf("expected loopback ok: %v", err)
	}
	if _, err := parseUpstream("https://localhost:8080"); err != nil {
		t.Fatalf("expected localhost ok: %v", err)
	}
	if _, err := parseUpstream("https://10.0.0.1:8080"); err == nil {
		t.Fatalf("expected non-loopback reject")
	}
}

func TestParseRelativeRejectsAbsolute(t *testing.T) {
	if _, err := parseRelative("https://example.com"); err == nil {
		t.Fatalf("expected absolute url reject")
	}
	if _, err := parseRelative("//example.com"); err == nil {
		t.Fatalf("expected scheme-relative reject")
	}
	if _, err := parseRelative("/ok/path?x=1"); err != nil {
		t.Fatalf("expected relative ok: %v", err)
	}
}

func TestReadTokenOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	dir := t.TempDir()
	dirs := appdirs.Dirs{RunDir: dir, Home: dir}
	path := filepath.Join(dir, "peer.token")
	if err := os.WriteFile(path, []byte("token"), 0o644); err != nil {
		t.Fatalf("write token: %v", err)
	}
	if _, err := readToken(dirs); err == nil {
		t.Fatalf("expected owner-only check to fail")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if tok, err := readToken(dirs); err != nil || tok != "token" {
		t.Fatalf("expected token read ok, got %v %q", err, tok)
	}
}
