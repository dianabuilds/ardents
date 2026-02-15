package api

import (
	"context"
	"testing"
	"time"
)

func TestServerRunStopsOnContextCancel(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
}

func TestServerInitFailsWhenTokenRequiredButMissing(t *testing.T) {
	t.Setenv("AIM_ENV", "production")
	t.Setenv("AIM_RPC_TOKEN", "")

	s := newTestServer(t)
	if s.initErr == nil {
		t.Fatal("expected init error when token is required but missing")
	}
}
