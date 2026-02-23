package daemonservice

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExportDiagnosticsBundleRedactsSensitiveData(t *testing.T) {
	t.Setenv("AIM_APP_VERSION", "app-v1 token=rpc_secret123")
	t.Setenv("AIM_NODE_VERSION", "node-v1")

	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, _, err := svc.CreateIdentity("pass"); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	svc.appendDiagnosticEvent("warn", "rpc", "warn token=rpc_secret123 identity=aim1abcdef", time.Now().UTC())
	svc.appendDiagnosticEvent("error", "storage", "private_key=abcd1234", time.Now().UTC())

	bundle, err := svc.ExportDiagnosticsBundle(60)
	if err != nil {
		t.Fatalf("export diagnostics: %v", err)
	}
	if bundle.SchemaVersion != diagnosticsExportSchemaVersion {
		t.Fatalf("unexpected schema version: %d", bundle.SchemaVersion)
	}
	if len(bundle.Events) < 2 {
		t.Fatalf("expected diagnostics events, got: %d", len(bundle.Events))
	}

	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal diagnostics bundle: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "rpc_secret123") {
		t.Fatalf("bundle leaks rpc token: %s", text)
	}
	if strings.Contains(text, "aim1abcdef") {
		t.Fatalf("bundle leaks raw identity id: %s", text)
	}
	if strings.Contains(strings.ToLower(text), "private_key=abcd1234") {
		t.Fatalf("bundle leaks private key material: %s", text)
	}
}

func TestExportDiagnosticsBundleFiltersByTimeWindow(t *testing.T) {
	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	now := time.Now().UTC()
	svc.appendDiagnosticEvent("warn", "old", "old event", now.Add(-40*time.Minute))
	svc.appendDiagnosticEvent("warn", "new", "new event", now.Add(-5*time.Minute))

	bundle, err := svc.ExportDiagnosticsBundle(10)
	if err != nil {
		t.Fatalf("export diagnostics: %v", err)
	}
	if len(bundle.Events) != 1 {
		t.Fatalf("expected one recent event, got: %d", len(bundle.Events))
	}
	if bundle.Events[0].Operation != "new" {
		t.Fatalf("unexpected event in window: %+v", bundle.Events[0])
	}
}
