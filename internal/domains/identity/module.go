//goland:noinspection GoNameStartsWithPackageName
package identity

import (
	"log/slog"

	"aim-chat/go-backend/internal/domains/contracts"
	identityusecase "aim-chat/go-backend/internal/domains/identity/usecase"
)

type Service = identityusecase.Service
type BackupExportResult = identityusecase.BackupExportResult

type Module struct {
	Service *Service
}

func NewService(
	identityManager contracts.IdentityDomain,
	identityState interface {
		Persist(identityManager contracts.IdentityDomain) error
	},
	messageStore contracts.MessageRepository,
	sessionManager contracts.SessionDomain,
	attachmentStore contracts.AttachmentRepository,
	logger *slog.Logger,
) *Service {
	return identityusecase.NewService(identityManager, identityState, messageStore, sessionManager, attachmentStore, logger)
}
