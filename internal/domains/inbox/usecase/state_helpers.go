package usecase

import (
	inboxmodel "aim-chat/go-backend/internal/domains/inbox/model"
	"aim-chat/go-backend/pkg/models"
	"time"
)

func CopyInboxState(inbox map[string][]models.Message) map[string][]models.Message {
	return inboxmodel.CloneMessageRequestInbox(inbox)
}

func CopyThread(messages []models.Message) []models.Message {
	return inboxmodel.CloneMessages(messages)
}

func ThreadHasMessage(messages []models.Message, messageID string) bool {
	return inboxmodel.HasMessageID(messages, messageID)
}

func NormalizeInboundTimestamp(ts time.Time) time.Time {
	return inboxmodel.NormalizeMessageTimestamp(ts)
}
