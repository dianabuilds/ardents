package privacy

import privacyusecase "aim-chat/go-backend/internal/domains/privacy/usecase"

type SettingsStateStore = privacyusecase.PrivacySettingsStateStore
type BlocklistStateStore = privacyusecase.BlocklistStateStore
type Service = privacyusecase.Service

type Module struct {
	Service *Service
}

func NewService(
	privacyState SettingsStateStore,
	blocklistState BlocklistStateStore,
	recordError func(string, error),
) *Service {
	return privacyusecase.NewService(privacyState, blocklistState, recordError)
}
