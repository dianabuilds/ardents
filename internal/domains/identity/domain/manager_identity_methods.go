package domain

import (
	"crypto/ed25519"
	"strings"

	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	"aim-chat/go-backend/pkg/models"
)

func (m *Manager) CreateIdentity(seedPassword string) (models.Identity, string, error) {
	mnemonic, keys, err := m.seeds.Create(seedPassword)
	if err != nil {
		return models.Identity{}, "", err
	}
	id, pub, err := FromKeys(keys)
	if err != nil {
		return models.Identity{}, "", err
	}

	m.mu.Lock()
	m.identity = models.Identity{
		ID:               id,
		SigningPublicKey: append([]byte(nil), pub...),
	}
	m.selfPriv = append(ed25519.PrivateKey(nil), keys.SigningPrivateKey...)
	m.contacts = make(map[string]models.Contact)
	m.revokedDevices = make(map[string]map[string]struct{})
	if err := m.initPrimaryDevice(); err != nil {
		m.mu.Unlock()
		return models.Identity{}, "", err
	}
	identity := models.Identity{
		ID:               m.identity.ID,
		SigningPublicKey: append([]byte(nil), m.identity.SigningPublicKey...),
	}
	m.mu.Unlock()
	return identity, mnemonic, nil
}

func (m *Manager) ImportIdentity(mnemonic, seedPassword string) (models.Identity, error) {
	_, keys, err := m.seeds.Import(mnemonic, seedPassword)
	if err != nil {
		return models.Identity{}, err
	}
	id, pub, err := FromKeys(keys)
	if err != nil {
		return models.Identity{}, err
	}

	m.mu.Lock()
	m.identity = models.Identity{
		ID:               id,
		SigningPublicKey: append([]byte(nil), pub...),
	}
	m.selfPriv = append(ed25519.PrivateKey(nil), keys.SigningPrivateKey...)
	m.contacts = make(map[string]models.Contact)
	m.revokedDevices = make(map[string]map[string]struct{})
	if err := m.initPrimaryDevice(); err != nil {
		m.mu.Unlock()
		return models.Identity{}, err
	}
	identity := models.Identity{
		ID:               m.identity.ID,
		SigningPublicKey: append([]byte(nil), m.identity.SigningPublicKey...),
	}
	m.mu.Unlock()
	return identity, nil
}

func (m *Manager) ExportSeed(seedPassword string) (string, error) {
	return m.seeds.Export(seedPassword)
}

func (m *Manager) ValidateMnemonic(mnemonic string) bool {
	return m.seeds.ValidateMnemonic(mnemonic)
}

func (m *Manager) ChangePassword(oldSeedPassword, newSeedPassword string) error {
	return m.seeds.ChangePassword(oldSeedPassword, newSeedPassword)
}

func (m *Manager) GetIdentity() models.Identity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return models.Identity{
		ID:               m.identity.ID,
		SigningPublicKey: append([]byte(nil), m.identity.SigningPublicKey...),
	}
}

func (m *Manager) SnapshotIdentityKeys() (publicKey []byte, privateKey []byte) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]byte(nil), m.identity.SigningPublicKey...), append([]byte(nil), m.selfPriv...)
}

func (m *Manager) RestoreIdentityPrivateKey(privateKey []byte) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return ErrInvalidContactCard
	}
	priv := ed25519.PrivateKey(append([]byte(nil), privateKey...))
	pub := priv.Public().(ed25519.PublicKey)
	id, err := identitypolicy.BuildIdentityID(pub)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.identity = models.Identity{
		ID:               id,
		SigningPublicKey: append([]byte(nil), pub...),
	}
	m.selfPriv = append(ed25519.PrivateKey(nil), priv...)
	m.contacts = make(map[string]models.Contact)
	m.revokedDevices = make(map[string]map[string]struct{})
	return m.initPrimaryDevice()
}

func (m *Manager) VerifyPassword(seedPassword string) error {
	_, err := m.seeds.Export(strings.TrimSpace(seedPassword))
	return err
}
