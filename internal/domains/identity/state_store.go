package identity

import identitydomain "aim-chat/go-backend/internal/domains/identity/domain"

type StateStore = identitydomain.StateStore

func NewStateStore() *StateStore {
	return identitydomain.NewStateStore()
}
