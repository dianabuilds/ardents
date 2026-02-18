package rpc

import (
	"encoding/json"
	"errors"
	"testing"
)

type wipeCapableMockService struct {
	channelMockService
	called bool
	token  string
	err    error
}

func (m *wipeCapableMockService) WipeData(consentToken string) (bool, error) {
	m.called = true
	m.token = consentToken
	if m.err != nil {
		return false, m.err
	}
	return true, nil
}

func TestDispatchRPCDataWipeSuccess(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &wipeCapableMockService{}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)
	params, _ := json.Marshal([]string{"I_UNDERSTAND_LOCAL_DATA_WIPE"})

	result, rpcErr := s.dispatchRPC("data.wipe", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if !svc.called {
		t.Fatalf("expected wipe method to be called")
	}
	if svc.token != "I_UNDERSTAND_LOCAL_DATA_WIPE" {
		t.Fatalf("unexpected consent token: %q", svc.token)
	}
	out, ok := result.(map[string]bool)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if !out["wiped"] {
		t.Fatalf("expected wiped=true")
	}
}

func TestDispatchRPCDataWipeUnsupported(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	s := newServerWithService(DefaultRPCAddr, &channelMockService{}, "", false)
	params, _ := json.Marshal([]string{"I_UNDERSTAND_LOCAL_DATA_WIPE"})

	_, rpcErr := s.dispatchRPC("data.wipe", params)
	if rpcErr == nil {
		t.Fatal("expected rpc error")
	}
	if rpcErr.Code != -32027 {
		t.Fatalf("unexpected rpc code: %d", rpcErr.Code)
	}
}

func TestDispatchRPCDataWipeServiceError(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &wipeCapableMockService{err: errors.New("boom")}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)
	params, _ := json.Marshal([]string{"I_UNDERSTAND_LOCAL_DATA_WIPE"})

	_, rpcErr := s.dispatchRPC("data.wipe", params)
	if rpcErr == nil {
		t.Fatal("expected rpc error")
	}
	if rpcErr.Code != -32027 {
		t.Fatalf("unexpected rpc code: %d", rpcErr.Code)
	}
}
