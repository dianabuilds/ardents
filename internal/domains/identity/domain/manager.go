package domain

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"sync"

	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	"aim-chat/go-backend/pkg/models"
)

var (
	ErrInvalidContactCard = identitypolicy.ErrInvalidContactCard
	ErrIdentityMismatch   = identitypolicy.ErrIdentityMismatch
	ErrInvalidContactID   = errors.New("invalid contact id")
	ErrContactKeyMismatch = errors.New("contact public key mismatch")
)

type Manager struct {
	mu             sync.RWMutex
	identity       models.Identity
	selfPriv       ed25519.PrivateKey
	contacts       map[string]models.Contact
	devices        map[string]devicePrivate
	activeDeviceID string
	revokedDevices map[string]map[string]struct{}
	seeds          *SeedManager
}

func newIdentityManager() (*Manager, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	id, err := identitypolicy.BuildIdentityID(pub)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		identity: models.Identity{
			ID:               id,
			SigningPublicKey: append([]byte(nil), pub...),
		},
		selfPriv:       append(ed25519.PrivateKey(nil), priv...),
		contacts:       make(map[string]models.Contact),
		devices:        make(map[string]devicePrivate),
		revokedDevices: make(map[string]map[string]struct{}),
		seeds:          NewSeedManager(),
	}
	if err := m.initPrimaryDevice(); err != nil {
		return nil, err
	}
	return m, nil
}
