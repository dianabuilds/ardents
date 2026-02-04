package quic

import (
	"os"
	"testing"
)

func withTempHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	prevHome := os.Getenv("ARDENTS_HOME")
	if err := os.Setenv("ARDENTS_HOME", tmp); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("ARDENTS_HOME", prevHome)
	})
	return tmp
}
