//go:build !windows

package perm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenOwnerOnly_EnforcesDirAndFilePerms(t *testing.T) {
	dir := t.TempDir()
	// Simulate an existing directory with permissive mode.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}

	path := filepath.Join(dir, "secret.txt")
	f, err := OpenOwnerOnly(path)
	if err != nil {
		t.Fatalf("OpenOwnerOnly: %v", err)
	}
	_ = f.Close()

	dinfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dinfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("dir not owner-only: %o", dinfo.Mode().Perm())
	}

	finfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if finfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("file not owner-only: %o", finfo.Mode().Perm())
	}
}

func TestEnsureOwnerOnly_FailsWhenDirTooPermissive(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	path := filepath.Join(dir, "peer.token")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := EnsureOwnerOnly(path); err == nil {
		t.Fatalf("expected owner-only check to fail for permissive dir")
	}
}
