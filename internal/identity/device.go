package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"aim-chat/go-backend/pkg/models"

	"golang.org/x/crypto/hkdf"
)

var (
	ErrDeviceNotFound    = errors.New("device not found")
	ErrDeviceRevoked     = errors.New("device revoked")
	ErrInvalidDeviceSig  = errors.New("invalid device signature")
	ErrInvalidDeviceCert = errors.New("invalid device certificate")
	ErrUnverifiedContact = errors.New("contact is not verified")
)

type devicePrivate struct {
	model models.Device
	priv  ed25519.PrivateKey
}

func (m *Manager) initPrimaryDevice() error {
	if len(m.selfPriv) < 32 {
		return errors.New("invalid identity private key")
	}
	seed := m.selfPriv[:32]
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	id := deviceIDFromPub(pub)
	certSig := ed25519.Sign(m.selfPriv, deviceCertBytes(m.identity.ID, id, pub))
	now := time.Now().UTC()
	m.devices = map[string]devicePrivate{
		id: {
			model: models.Device{
				ID:        id,
				Name:      "primary",
				PublicKey: append([]byte(nil), pub...),
				CertSig:   certSig,
				CreatedAt: now,
			},
			priv: append(ed25519.PrivateKey(nil), priv...),
		},
	}
	m.activeDeviceID = id
	return nil
}

func (m *Manager) ListDevices() []models.Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.Device, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, cloneDevice(d.model))
	}
	return out
}

func (m *Manager) AddDevice(name string) (models.Device, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "device"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	index := len(m.devices) + 1
	seed, err := deriveDeviceSeed(m.selfPriv[:32], index)
	if err != nil {
		return models.Device{}, err
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	id := deviceIDFromPub(pub)
	now := time.Now().UTC()
	device := models.Device{
		ID:        id,
		Name:      name,
		PublicKey: append([]byte(nil), pub...),
		CertSig:   ed25519.Sign(m.selfPriv, deviceCertBytes(m.identity.ID, id, pub)),
		CreatedAt: now,
	}
	m.devices[id] = devicePrivate{
		model: device,
		priv:  append(ed25519.PrivateKey(nil), priv...),
	}
	return cloneDevice(device), nil
}

func (m *Manager) RevokeDevice(deviceID string) (models.DeviceRevocation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[deviceID]
	if !ok {
		return models.DeviceRevocation{}, ErrDeviceNotFound
	}
	if d.model.IsRevoked {
		return m.buildRevocationLocked(deviceID), nil
	}
	d.model.IsRevoked = true
	d.model.RevokedAt = time.Now().UTC()
	m.devices[deviceID] = d
	return m.buildRevocationLocked(deviceID), nil
}

func (m *Manager) ActiveDeviceAuth(payload []byte) (models.Device, []byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.devices[m.activeDeviceID]
	if !ok {
		return models.Device{}, nil, ErrDeviceNotFound
	}
	if d.model.IsRevoked {
		return models.Device{}, nil, ErrDeviceRevoked
	}
	sig := ed25519.Sign(d.priv, payload)
	return cloneDevice(d.model), sig, nil
}

func (m *Manager) VerifyInboundDevice(contactID string, device models.Device, payload, sig []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	contact, ok := m.contacts[contactID]
	if !ok {
		return ErrInvalidContactCard
	}
	if len(contact.PublicKey) != ed25519.PublicKeySize {
		return ErrUnverifiedContact
	}
	revokedSet := m.revokedDevices[contactID]
	if revokedSet != nil {
		if _, revoked := revokedSet[device.ID]; revoked {
			return ErrDeviceRevoked
		}
	}
	if !ed25519.Verify(contact.PublicKey, deviceCertBytes(contactID, device.ID, device.PublicKey), device.CertSig) {
		return ErrInvalidDeviceCert
	}
	if !ed25519.Verify(device.PublicKey, payload, sig) {
		return ErrInvalidDeviceSig
	}
	return nil
}

func (m *Manager) ApplyDeviceRevocation(contactID string, rev models.DeviceRevocation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	contact, ok := m.contacts[contactID]
	if !ok {
		return ErrInvalidContactCard
	}
	if len(contact.PublicKey) != ed25519.PublicKeySize {
		return nil
	}
	if rev.IdentityID != contactID {
		return ErrIdentityMismatch
	}
	if !ed25519.Verify(contact.PublicKey, deviceRevocationBytes(rev.IdentityID, rev.DeviceID, rev.Timestamp), rev.Signature) {
		return ErrInvalidDeviceSig
	}
	if m.revokedDevices == nil {
		m.revokedDevices = make(map[string]map[string]struct{})
	}
	if m.revokedDevices[contactID] == nil {
		m.revokedDevices[contactID] = make(map[string]struct{})
	}
	m.revokedDevices[contactID][rev.DeviceID] = struct{}{}
	return nil
}

func (m *Manager) buildRevocationLocked(deviceID string) models.DeviceRevocation {
	now := time.Now().UTC()
	return models.DeviceRevocation{
		IdentityID: m.identity.ID,
		DeviceID:   deviceID,
		Timestamp:  now,
		Signature:  ed25519.Sign(m.selfPriv, deviceRevocationBytes(m.identity.ID, deviceID, now)),
	}
}

func deviceIDFromPub(pub []byte) string {
	sum := sha256.Sum256(pub)
	return "dev1_" + hex.EncodeToString(sum[:8])
}

func deviceCertBytes(identityID, deviceID string, pub []byte) []byte {
	b := make([]byte, 0, len(identityID)+len(deviceID)+len(pub)+2)
	b = append(b, []byte(identityID)...)
	b = append(b, 0)
	b = append(b, []byte(deviceID)...)
	b = append(b, 0)
	b = append(b, pub...)
	return b
}

func deviceRevocationBytes(identityID, deviceID string, ts time.Time) []byte {
	return []byte(fmt.Sprintf("%s:%s:%d", identityID, deviceID, ts.UnixNano()))
}

func deriveDeviceSeed(masterSeed []byte, index int) ([]byte, error) {
	reader := hkdf.New(sha256.New, masterSeed, nil, []byte(fmt.Sprintf("aim/device/%d", index)))
	out := make([]byte, 32)
	_, err := reader.Read(out)
	return out, err
}

func cloneDevice(d models.Device) models.Device {
	return models.Device{
		ID:        d.ID,
		Name:      d.Name,
		PublicKey: append([]byte(nil), d.PublicKey...),
		CertSig:   append([]byte(nil), d.CertSig...),
		CreatedAt: d.CreatedAt,
		IsRevoked: d.IsRevoked,
		RevokedAt: d.RevokedAt,
	}
}
