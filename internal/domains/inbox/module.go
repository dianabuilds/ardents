//goland:noinspection GoNameStartsWithPackageName
package inbox

import (
	inboxusecase "aim-chat/go-backend/internal/domains/inbox/usecase"
	"aim-chat/go-backend/pkg/models"
	"time"
)

type Service = inboxusecase.Service

type Module struct {
	Service *Service
}

func CopyInboxState(inbox map[string][]models.Message) map[string][]models.Message {
	return inboxusecase.CopyInboxState(inbox)
}

func CopyThread(messages []models.Message) []models.Message {
	return inboxusecase.CopyThread(messages)
}

func ThreadHasMessage(messages []models.Message, messageID string) bool {
	return inboxusecase.ThreadHasMessage(messages, messageID)
}

func NormalizeInboundTimestamp(ts time.Time) time.Time {
	return inboxusecase.NormalizeInboundTimestamp(ts)
}
