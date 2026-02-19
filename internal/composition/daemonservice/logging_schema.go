package daemonservice

import (
	"strings"
)

const daemonComponentName = "daemonservice"

func messageCorrelationID(messageID, contactID string) string {
	trimmedMessageID := strings.TrimSpace(messageID)
	trimmedContactID := strings.TrimSpace(contactID)
	switch {
	case trimmedMessageID != "" && trimmedContactID != "":
		return trimmedContactID + ":" + trimmedMessageID
	case trimmedMessageID != "":
		return trimmedMessageID
	case trimmedContactID != "":
		return trimmedContactID
	default:
		return "n/a"
	}
}

func (s *Service) logInfo(operation, correlationID, message string, attrs ...any) {
	base := []any{
		"component", daemonComponentName,
		"operation", strings.TrimSpace(operation),
		"correlation_id", strings.TrimSpace(correlationID),
	}
	s.logger.Info(message, append(base, attrs...)...)
}

func (s *Service) logWarn(operation, correlationID, message string, attrs ...any) {
	base := []any{
		"component", daemonComponentName,
		"operation", strings.TrimSpace(operation),
		"correlation_id", strings.TrimSpace(correlationID),
	}
	s.logger.Warn(message, append(base, attrs...)...)
}

func (s *Service) recordErrorWithContext(category string, err error, operation, correlationID string, attrs ...any) {
	if err == nil {
		return
	}
	s.metrics.RecordError(category)
	base := []any{
		"component", daemonComponentName,
		"operation", strings.TrimSpace(operation),
		"category", strings.TrimSpace(category),
		"correlation_id", strings.TrimSpace(correlationID),
		"error", err.Error(),
	}
	s.logger.Error("service error", append(base, attrs...)...)
}
