package identity

import (
	"aim-chat/go-backend/internal/domains/contracts"
	legacyidentity "aim-chat/go-backend/internal/identity"
)

func NewManager() (contracts.IdentityDomain, error) {
	return legacyidentity.NewManager()
}
