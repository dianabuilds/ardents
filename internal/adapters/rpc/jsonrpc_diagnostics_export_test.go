package rpc

import (
	"testing"
	"time"

	"aim-chat/go-backend/pkg/models"
)

type diagnosticsExportMockService struct {
	channelMockService
	called bool
	result models.DiagnosticsExportPackage
	err    error
}

func (m *diagnosticsExportMockService) ExportDiagnosticsBundle(_ int) (models.DiagnosticsExportPackage, error) {
	m.called = true
	if m.err != nil {
		return models.DiagnosticsExportPackage{}, m.err
	}
	return m.result, nil
}

func TestDispatchRPCDiagnosticsExportSuccess(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	expected := models.DiagnosticsExportPackage{
		SchemaVersion: 1,
		ExportedAt:    time.Now().UTC(),
		AppVersion:    "app-v1",
		NodeVersion:   "node-v1",
	}
	svc := &diagnosticsExportMockService{result: expected}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	result, rpcErr := s.dispatchRPC("diagnostics.export", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if !svc.called {
		t.Fatal("expected diagnostics export to be called")
	}
	got, ok := result.(models.DiagnosticsExportPackage)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if got.SchemaVersion != expected.SchemaVersion || got.AppVersion != expected.AppVersion || got.NodeVersion != expected.NodeVersion {
		t.Fatalf("unexpected export payload: %+v", got)
	}
}

func TestDispatchRPCDiagnosticsExportUnsupported(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	s := newServerWithService(DefaultRPCAddr, &channelMockService{}, "", false)
	_, rpcErr := s.dispatchRPC("diagnostics.export", nil)
	if rpcErr == nil {
		t.Fatal("expected rpc error")
	}
	if rpcErr.Code != -32071 {
		t.Fatalf("unexpected rpc code: %d", rpcErr.Code)
	}
}
