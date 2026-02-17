package messaging

import messagingusecase "aim-chat/go-backend/internal/domains/messaging/usecase"

type Service = messagingusecase.Service
type ServiceDeps = messagingusecase.ServiceDeps

func NewService(deps ServiceDeps) *Service {
	return messagingusecase.NewService(deps)
}
