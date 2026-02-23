package domain

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"time"

	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	"aim-chat/go-backend/pkg/models"
)

func (m *Manager) AddContact(card models.ContactCard) error {
	if ok, err := identitypolicy.VerifyContactCard(card); err != nil || !ok {
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

func (m *Manager) VerifyContactCard(card models.ContactCard) (bool, error) {
	return identitypolicy.VerifyContactCard(card)
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
	return identitypolicy.SignContactCard(m.identity.ID, displayName, pub, priv)
}
