package identity

import (
	"aim-chat/go-backend/internal/domains/contracts"
	identitydomain "aim-chat/go-backend/internal/domains/identity/domain"
)

func NewManager() (contracts.IdentityDomain, error) {
	return identitydomain.NewManager()
}
