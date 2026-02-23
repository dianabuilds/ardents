package identity

import (
	identityusecase "aim-chat/go-backend/internal/domains/identity/usecase"
	"aim-chat/go-backend/pkg/models"
)

func CreateIdentity(seedPassword string, identity interface {
	CreateIdentity(seedPassword string) (models.Identity, string, error)
}, persist func() error) (models.Identity, string, error) {
	return identityusecase.CreateIdentity(seedPassword, identity, persist)
}

func ImportIdentity(mnemonic, seedPassword string, identity interface {
	ImportIdentity(mnemonic, seedPassword string) (models.Identity, error)
}, persist func() error) (models.Identity, error) {
	return identityusecase.ImportIdentity(mnemonic, seedPassword, identity, persist)
}
