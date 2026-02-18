package privacylog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
)

const redactedValue = "[REDACTED]"

var (
	bootNonce          = randomNonce()
	disallowedPlainIDs = map[string]struct{}{
		"contact_id":  {},
		"message_id":  {},
		"group_id":    {},
		"event_id":    {},
		"identity_id": {},
	}
	sensitiveKeyParts = []string{"token", "secret", "password", "passphrase", "authorization", "auth"}
)

type SanitizingHandler struct {
	next slog.Handler
}

func WrapHandler(next slog.Handler) slog.Handler {
	if next == nil {
		return nil
	}
	return &SanitizingHandler{next: next}
}

func (h *SanitizingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *SanitizingHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	rec.Attrs(func(attr slog.Attr) bool {
		out.AddAttrs(SanitizeAttr(attr))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *SanitizingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h.next.WithAttrs(sanitizeAttrs(attrs))
	return &SanitizingHandler{next: next}
}

func (h *SanitizingHandler) WithGroup(name string) slog.Handler {
	return &SanitizingHandler{next: h.next.WithGroup(name)}
}

func SanitizeAttr(attr slog.Attr) slog.Attr {
	key := strings.TrimSpace(attr.Key)
	lowerKey := strings.ToLower(key)
	if isSensitiveKey(lowerKey) {
		return slog.String(key, redactedValue)
	}
	if shouldFingerprintKey(lowerKey) {
		return slog.String(fingerprintKeyName(key), FingerprintID(valueToString(attr.Value)))
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		return slog.Any(key, sanitizeGroupValue(group))
	}
	return attr
}

func SanitizeArgs(args ...any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, 0, len(args))
	for i := 0; i < len(args); i++ {
		key, ok := args[i].(string)
		if !ok || i+1 >= len(args) {
			out = append(out, args[i])
			continue
		}
		value := args[i+1]
		i++
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		switch {
		case isSensitiveKey(lowerKey):
			out = append(out, key, redactedValue)
		case shouldFingerprintKey(lowerKey):
			out = append(out, fingerprintKeyName(key), FingerprintID(fmt.Sprint(value)))
		default:
			out = append(out, key, value)
		}
	}
	return out
}

func FingerprintID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed + "|" + bootNonce))
	return "fp_" + hex.EncodeToString(sum[:8])
}

func sanitizeAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, SanitizeAttr(attr))
	}
	return out
}

func sanitizeGroupValue(attrs []slog.Attr) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, attr := range sanitizeAttrs(attrs) {
		switch attr.Value.Kind() {
		case slog.KindString:
			out[attr.Key] = attr.Value.String()
		case slog.KindInt64:
			out[attr.Key] = attr.Value.Int64()
		case slog.KindUint64:
			out[attr.Key] = attr.Value.Uint64()
		case slog.KindFloat64:
			out[attr.Key] = attr.Value.Float64()
		case slog.KindBool:
			out[attr.Key] = attr.Value.Bool()
		case slog.KindDuration:
			out[attr.Key] = attr.Value.Duration().String()
		case slog.KindTime:
			out[attr.Key] = attr.Value.Time().UTC().Format("2006-01-02T15:04:05.000000000Z")
		default:
			out[attr.Key] = attr.Value.Any()
		}
	}
	return out
}

func shouldFingerprintKey(key string) bool {
	if _, ok := disallowedPlainIDs[key]; ok {
		return true
	}
	return key == "actor_id" || key == "member_id" || key == "device_id"
}

func fingerprintKeyName(key string) string {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(key)), "_fp") {
		return key
	}
	return key + "_fp"
}

func isSensitiveKey(key string) bool {
	for _, part := range sensitiveKeyParts {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func valueToString(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return fmt.Sprintf("%d", v.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", v.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", v.Float64())
	case slog.KindBool:
		return fmt.Sprintf("%t", v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().UTC().Format("2006-01-02T15:04:05.000000000Z")
	default:
		return fmt.Sprint(v.Any())
	}
}

func randomNonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "fallback_nonce"
	}
	return hex.EncodeToString(buf)
}
