package domain

func NewManager() (*Manager, error) {
	return newIdentityManager()
}
