package rpc

import (
	"encoding/json"
	"testing"
)

func TestDispatchRPCGroupsKillSwitchBlocksGroupAndChannelMethods(t *testing.T) {
	t.Setenv("AIM_ENV", "test")
	t.Setenv("AIM_GROUPS_ENABLED", "false")

	s := newServerWithService(DefaultRPCAddr, &channelMockService{}, "", false)

	for _, method := range []string{"group.create", "channel.create"} {
		params, _ := json.Marshal([]string{"news"})
		result, rpcErr := s.dispatchRPC(method, params)
		if result != nil {
			t.Fatalf("expected nil result for %s when groups are disabled", method)
		}
		if rpcErr == nil {
			t.Fatalf("expected rpc error for %s when groups are disabled", method)
		}
		if rpcErr.Code != -32199 {
			t.Fatalf("expected code -32199 for %s, got %d", method, rpcErr.Code)
		}
	}
}

func TestDispatchRPCGroupsKillSwitchDoesNotBlockNonGroupMethods(t *testing.T) {
	t.Setenv("AIM_ENV", "test")
	t.Setenv("AIM_GROUPS_ENABLED", "false")

	s := newServerWithService(DefaultRPCAddr, &channelMockService{}, "", false)
	result, rpcErr := s.dispatchRPC("rpc.version", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected rpc.version result")
	}
}
