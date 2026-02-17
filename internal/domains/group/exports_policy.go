package group

import (
	grouppolicy "aim-chat/go-backend/internal/domains/group/policy"
	"time"
)

type AbuseProtection = grouppolicy.AbuseProtection

func NewAbuseProtectionFromEnv() *AbuseProtection {
	return grouppolicy.NewAbuseProtectionFromEnv()
}

type InboundGroupMessageRejectReason = grouppolicy.InboundGroupMessageRejectReason

const (
	ReplayWindow = grouppolicy.ReplayWindow
)

func BuildReplayGuardKey(kind, groupID, senderDeviceID, uniqueID string) (string, error) {
	return grouppolicy.BuildReplayGuardKey(kind, groupID, senderDeviceID, uniqueID)
}

func ValidateReplayOccurredAt(occurredAt, now time.Time) error {
	return grouppolicy.ValidateReplayOccurredAt(occurredAt, now)
}
