package api

import (
	"os"
	"testing"

	"aim-chat/go-backend/internal/waku"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	if _, ok := os.LookupEnv("AIM_ENV"); !ok {
		t.Setenv("AIM_ENV", "test")
	}
	svc, err := NewServiceForDaemonWithDataDir(waku.DefaultConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("new daemon service failed: %v", err)
	}
	return NewServerWithService(defaultRPCAddr, svc)
}
