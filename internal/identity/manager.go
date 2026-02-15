package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"

	"aim-chat/go-backend/pkg/models"
)

var (
	ErrInvalidContactCard = errors.New("invalid contact card")
	ErrIdentityMismatch   = errors.New("identity_id does not match public key")
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

func NewManager() (*Manager, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	id, err := BuildIdentityID(pub)
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

func (m *Manager) CreateIdentity(password string) (models.Identity, string, error) {
	mnemonic, keys, err := m.seeds.Create(password)
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

func (m *Manager) ImportIdentity(mnemonic, password string) (models.Identity, error) {
	_, keys, err := m.seeds.Import(mnemonic, password)
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

func (m *Manager) ExportSeed(password string) (string, error) {
	return m.seeds.Export(password)
}

func (m *Manager) ValidateMnemonic(mnemonic string) bool {
	return m.seeds.ValidateMnemonic(mnemonic)
}

func (m *Manager) ChangePassword(oldPassword, newPassword string) error {
	return m.seeds.ChangePassword(oldPassword, newPassword)
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
	id, err := BuildIdentityID(pub)
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
	m.revokedDevices = make(map[string]map[string]struct{})
	return m.initPrimaryDevice()
}

func (m *Manager) AddContact(card models.ContactCard) error {
	if ok, err := VerifyContactCard(card); err != nil || !ok {
		if err != nil {
			return err
		}
		return ErrInvalidContactCard
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.contacts[card.IdentityID]; ok && len(existing.PublicKey) == ed25519.PublicKeySize {
		if !bytes.Equal(existing.PublicKey, card.PublicKey) {
			return ErrContactKeyMismatch
		}
	}
	m.contacts[card.IdentityID] = models.Contact{
		ID:          card.IdentityID,
		DisplayName: card.DisplayName,
		PublicKey:   append([]byte(nil), card.PublicKey...),
		AddedAt:     time.Now(),
	}
	return nil
}

func (m *Manager) AddContactByIdentityID(contactID, displayName string) error {
	contactID = strings.TrimSpace(contactID)
	displayName = strings.TrimSpace(displayName)
	if !strings.HasPrefix(contactID, "aim1") || len(contactID) < 12 {
		return ErrInvalidContactID
	}
	if displayName == "" {
		displayName = contactID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, exists := m.contacts[contactID]
	publicKey := []byte(nil)
	if exists && len(existing.PublicKey) == ed25519.PublicKeySize {
		publicKey = append([]byte(nil), existing.PublicKey...)
	}
	m.contacts[contactID] = models.Contact{
		ID:          contactID,
		DisplayName: displayName,
		PublicKey:   publicKey,
		AddedAt:     time.Now(),
	}
	return nil
}

func (m *Manager) RemoveContact(contactID string) error {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return ErrInvalidContactID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.contacts[contactID]; !ok {
		return ErrInvalidContactID
	}
	delete(m.contacts, contactID)
	return nil
}

func (m *Manager) VerifyPassword(password string) error {
	_, err := m.seeds.Export(strings.TrimSpace(password))
	return err
}

func (m *Manager) VerifyContactCard(card models.ContactCard) (bool, error) {
	return VerifyContactCard(card)
}

func (m *Manager) Contacts() []models.Contact {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.Contact, 0, len(m.contacts))
	for _, c := range m.contacts {
		out = append(out, c)
	}
	return out
}

func (m *Manager) HasContact(contactID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.contacts[contactID]
	return ok
}

func (m *Manager) HasVerifiedContact(contactID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	contact, ok := m.contacts[contactID]
	if !ok {
		return false
	}
	return len(contact.PublicKey) == ed25519.PublicKeySize
}

func (m *Manager) ContactPublicKey(contactID string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	contact, ok := m.contacts[contactID]
	if !ok || len(contact.PublicKey) != ed25519.PublicKeySize {
		return nil, false
	}
	return append([]byte(nil), contact.PublicKey...), true
}

func (m *Manager) SelfContactCard(displayName string) (models.ContactCard, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pub := ed25519.PublicKey(append([]byte(nil), m.identity.SigningPublicKey...))
	priv := ed25519.PrivateKey(append([]byte(nil), m.selfPriv...))
	return SignContactCard(m.identity.ID, displayName, pub, priv)
}

func SignContactCard(identityID, displayName string, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) (models.ContactCard, error) {
	if privateKey == nil || publicKey == nil {
		return models.ContactCard{}, ErrInvalidContactCard
	}
	card := models.ContactCard{
		IdentityID:  identityID,
		DisplayName: displayName,
		PublicKey:   append([]byte(nil), publicKey...),
	}
	if ok, err := VerifyIdentityID(identityID, publicKey); err != nil || !ok {
		if err != nil {
			return models.ContactCard{}, err
		}
		return models.ContactCard{}, ErrIdentityMismatch
	}
	card.Signature = ed25519.Sign(privateKey, contactCardSigningBytes(card))
	return card, nil
}

func VerifyContactCard(card models.ContactCard) (bool, error) {
	if len(card.PublicKey) != ed25519.PublicKeySize || len(card.Signature) != ed25519.SignatureSize {
		return false, ErrInvalidContactCard
	}
	ok, err := VerifyIdentityID(card.IdentityID, card.PublicKey)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrIdentityMismatch
	}
	return ed25519.Verify(card.PublicKey, contactCardSigningBytes(card), card.Signature), nil
}

func contactCardSigningBytes(card models.ContactCard) []byte {
	// Canonical and deterministic byte encoding for signatures.
	b := make([]byte, 0, len(card.IdentityID)+len(card.DisplayName)+len(card.PublicKey)+2)
	b = append(b, []byte(card.IdentityID)...)
	b = append(b, 0)
	b = append(b, []byte(card.DisplayName)...)
	b = append(b, 0)
	b = append(b, card.PublicKey...)
	return b
}
