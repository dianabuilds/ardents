package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	ReplayWindow     = 30 * time.Minute
	ReplayFutureSkew = 2 * time.Minute
)

func DeriveRecipientMessageID(eventID, recipientID string) string {
	input := strings.TrimSpace(eventID) + "|" + strings.TrimSpace(recipientID)
	sum := sha256.Sum256([]byte(input))
	return "gmsg_" + hex.EncodeToString(sum[:12])
}

func CorrelationID(groupID, eventID string) string {
	groupID = strings.TrimSpace(groupID)
	eventID = strings.TrimSpace(eventID)
	if groupID == "" && eventID == "" {
		return "group:unknown"
	}
	if eventID == "" {
		return "group:" + groupID
	}
	return "group:" + groupID + ":event:" + eventID
}

func BuildReplayGuardKey(kind, groupID, senderDeviceID, uniqueID string) (string, error) {
	groupID = strings.TrimSpace(groupID)
	senderDeviceID = strings.TrimSpace(senderDeviceID)
	uniqueID = strings.TrimSpace(uniqueID)
	kind = strings.TrimSpace(kind)
	if groupID == "" || senderDeviceID == "" || uniqueID == "" || kind == "" {
		return "", ErrInvalidGroupEventPayload
	}
	return kind + "|" + groupID + "|" + senderDeviceID + "|" + uniqueID, nil
}

func ValidateReplayOccurredAt(occurredAt, now time.Time) error {
	if occurredAt.IsZero() {
		return nil
	}
	lowerBound := now.Add(-ReplayWindow)
	upperBound := now.Add(ReplayFutureSkew)
	if occurredAt.Before(lowerBound) || occurredAt.After(upperBound) {
		return ErrOutOfOrderGroupEvent
	}
	return nil
}
