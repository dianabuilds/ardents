package daemonservice

import (
	"os"
	"regexp"
	"strings"
	"time"

	"aim-chat/go-backend/pkg/models"
)

const (
	diagnosticsExportSchemaVersion = 1
	diagnosticEventsRingLimit      = 512
)

var (
	diagnosticTokenPattern    = regexp.MustCompile(`(?i)\b(rpc_[a-z0-9._-]+)\b`)
	diagnosticIdentityPattern = regexp.MustCompile(`\baim1[0-9a-zA-Z]+\b`)
	diagnosticSecretKVPattern = regexp.MustCompile(`(?i)\b(token|secret|password|passphrase|private[_-]?key)\s*[:=]\s*([^\s,;]+)`)
)

func (s *Service) ExportDiagnosticsBundle(windowMinutes int) (models.DiagnosticsExportPackage, error) {
	if windowMinutes <= 0 {
		windowMinutes = envBoundedIntWithFallback("AIM_DIAGNOSTICS_EXPORT_WINDOW_MIN", 15, 1, 24*60)
	}
	now := time.Now().UTC()
	metrics := s.GetMetrics()
	preset := s.GetBlobNodePreset()

	return models.DiagnosticsExportPackage{
		SchemaVersion: diagnosticsExportSchemaVersion,
		ExportedAt:    now,
		AppVersion:    envStringOrUnknown("AIM_APP_VERSION"),
		NodeVersion:   envStringOrUnknown("AIM_NODE_VERSION"),
		ProfileConfig: models.DiagnosticsProfileConfig{
			ProfileID:                  sanitizeDiagnosticText(preset.ProfileID),
			Preset:                     sanitizeDiagnosticText(preset.Preset),
			RelayEnabled:               preset.RelayEnabled,
			PublicDiscoveryEnabled:     preset.PublicDiscoveryEnabled,
			PublicServingEnabled:       preset.PublicServingEnabled,
			PublicStoreEnabled:         preset.PublicStoreEnabled,
			PersonalStoreEnabled:       preset.PersonalStoreEnabled,
			ServeBandwidthSoftKBps:     preset.ServeBandwidthSoftKBps,
			ServeBandwidthHardKBps:     preset.ServeBandwidthHardKBps,
			ServeMaxConcurrent:         preset.ServeMaxConcurrent,
			ServeRequestsPerMinPerPeer: preset.ServeRequestsPerMinPerPeer,
			PublicEphemeralCacheMaxMB:  preset.PublicEphemeralCacheMaxMB,
			PublicEphemeralCacheTTLMin: preset.PublicEphemeralCacheTTLMin,
			FetchBandwidthKBps:         preset.FetchBandwidthKBps,
		},
		Metrics: models.DiagnosticsAggregatedMetrics{
			PeerCount:           metrics.PeerCount,
			PendingQueueSize:    metrics.PendingQueueSize,
			RetryAttemptsTotal:  metrics.RetryAttemptsTotal,
			NotificationBacklog: metrics.NotificationBacklog,
			ErrorCounters:       metrics.ErrorCounters,
			BlobFetchStats:      metrics.BlobFetchStats,
			DiskUsageByClass:    metrics.DiskUsageByClass,
		},
		Events: s.diagnosticsEventsSince(now.Add(-time.Duration(windowMinutes) * time.Minute)),
	}, nil
}

func (s *Service) appendDiagnosticEvent(level, operation, message string, occurredAt time.Time) {
	if s == nil || s.diagEventsMu == nil {
		return
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if level != "warn" && level != "error" {
		return
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	event := diagnosticEventEntry{
		Level:      level,
		OccurredAt: occurredAt,
		Operation:  sanitizeDiagnosticText(operation),
		Message:    sanitizeDiagnosticText(message),
	}
	s.diagEventsMu.Lock()
	defer s.diagEventsMu.Unlock()
	s.diagEvents = append(s.diagEvents, event)
	if len(s.diagEvents) > diagnosticEventsRingLimit {
		s.diagEvents = append([]diagnosticEventEntry(nil), s.diagEvents[len(s.diagEvents)-diagnosticEventsRingLimit:]...)
	}
}

func (s *Service) diagnosticsEventsSince(cutoff time.Time) []models.DiagnosticsEvent {
	if s == nil || s.diagEventsMu == nil {
		return nil
	}
	s.diagEventsMu.Lock()
	events := append([]diagnosticEventEntry(nil), s.diagEvents...)
	s.diagEventsMu.Unlock()

	out := make([]models.DiagnosticsEvent, 0, len(events))
	for _, event := range events {
		if !cutoff.IsZero() && event.OccurredAt.Before(cutoff) {
			continue
		}
		out = append(out, models.DiagnosticsEvent{
			Level:      event.Level,
			OccurredAt: event.OccurredAt,
			Operation:  event.Operation,
			Message:    event.Message,
		})
	}
	return out
}

func sanitizeDiagnosticText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = diagnosticSecretKVPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := diagnosticSecretKVPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return "[REDACTED]"
		}
		return parts[1] + "=[REDACTED]"
	})
	value = diagnosticTokenPattern.ReplaceAllString(value, "rpc_[REDACTED]")
	value = diagnosticIdentityPattern.ReplaceAllString(value, "aim1[REDACTED]")
	return value
}

func envStringOrUnknown(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "unknown"
	}
	return sanitizeDiagnosticText(value)
}
