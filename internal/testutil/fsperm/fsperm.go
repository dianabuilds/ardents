package fsperm

import (
	"os"
	"runtime"
	"testing"
)

// AssertPrivateDirPerm verifies that dir exists and is private enough for persisted state.
func AssertPrivateDirPerm(t testing.TB, dir string) {
	t.Helper()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir failed: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory, got file: %s", dir)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("expected dir perm 0700, got %04o for %s", perm, dir)
	}
}
