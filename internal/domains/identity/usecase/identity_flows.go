package usecase

import (
	"strings"

	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	identityports "aim-chat/go-backend/internal/domains/identity/ports"
	"aim-chat/go-backend/pkg/models"
)

func CreateAccount(seedPassword string, identity identityports.CreateAccountIdentity) (models.Account, error) {
	created, _, err := identity.CreateIdentity(seedPassword)
	if err != nil {
		return models.Account{}, err
	}
	return models.Account{
		ID:                created.ID,
		IdentityPublicKey: created.SigningPublicKey,
	}, nil
}

func Login(identityID, seedPassword string, identity identityports.AccountIdentityAccess) error {
	current := identity.GetIdentity()
	if err := identitypolicy.ValidateLoginInput(identityID, seedPassword, current.ID); err != nil {
		return err
	}
	return identity.VerifyPassword(seedPassword)
}

func CreateIdentity(
	seedPassword string,
	identity identityports.CreateIdentityAccess,
	persist func() error,
) (models.Identity, string, error) {
	created, mnemonic, err := identity.CreateIdentity(strings.TrimSpace(seedPassword))
	if err != nil {
		return models.Identity{}, "", err
	}
	if err := persist(); err != nil {
		return models.Identity{}, "", err
	}
	return created, mnemonic, nil
}

func ImportIdentity(
	mnemonic, seedPassword string,
	identity identityports.ImportIdentityAccess,
	persist func() error,
) (models.Identity, error) {
	created, err := identity.ImportIdentity(strings.TrimSpace(mnemonic), strings.TrimSpace(seedPassword))
	if err != nil {
		return models.Identity{}, err
	}
	if err := persist(); err != nil {
		return models.Identity{}, err
	}
	return created, nil
}
