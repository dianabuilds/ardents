package privacylog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSanitizeArgsFingerprintsDisallowedIDs(t *testing.T) {
	args := SanitizeArgs(
		"contact_id", "aim1_contact_123",
		"message_id", "msg_123",
		"kind", "private",
	)
	if len(args) != 6 {
		t.Fatalf("unexpected args length: %d", len(args))
	}
	if got := args[0]; got != "contact_id_fp" {
		t.Fatalf("unexpected key: %v", got)
	}
	if got := args[1].(string); !strings.HasPrefix(got, "fp_") {
		t.Fatalf("unexpected fingerprint value: %q", got)
	}
	if got := args[4]; got != "kind" {
		t.Fatalf("expected untouched key, got %v", got)
	}
}

func TestSanitizingHandlerRedactsSensitiveAndIDs(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(WrapHandler(base))
	logger.Info("test", "contact_id", "aim1_contact", "rpc_token", "secret", "status", "ok")

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("decode log json: %v", err)
	}
	if _, ok := payload["contact_id"]; ok {
		t.Fatal("contact_id should not be present")
	}
	if _, ok := payload["contact_id_fp"]; !ok {
		t.Fatal("contact_id_fp should be present")
	}
	if got, _ := payload["rpc_token"].(string); got != redactedValue {
		t.Fatalf("expected redacted token, got %q", got)
	}
}

func TestSanitizingHandlerImplementsSlogHandlerContract(t *testing.T) {
	var buf bytes.Buffer
	h := WrapHandler(slog.NewJSONHandler(&buf, nil))
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("expected handler enabled for info")
	}
	rec := slog.NewRecord(time.Now().UTC(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.String("group_id", "g1"))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("handle failed: %v", err)
	}
	if !strings.Contains(buf.String(), "group_id_fp") {
		t.Fatalf("expected sanitized group_id key, got %s", buf.String())
	}
}
