package domain

import (
	"crypto/ed25519"
	"encoding/json"

	"aim-chat/go-backend/pkg/models"
)

type persistedRuntimeState struct {
	Contacts       []models.Contact    `json:"contacts,omitempty"`
	Devices        []persistedDevice   `json:"devices,omitempty"`
	ActiveDeviceID string              `json:"active_device_id,omitempty"`
	RevokedDevices map[string][]string `json:"revoked_devices,omitempty"`
}

type persistedDevice struct {
	Model      models.Device `json:"model"`
	PrivateKey []byte        `json:"private_key"`
}

func (m *Manager) SnapshotRuntimeStateJSON() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state := persistedRuntimeState{
		Contacts:       make([]models.Contact, 0, len(m.contacts)),
		Devices:        make([]persistedDevice, 0, len(m.devices)),
		ActiveDeviceID: m.activeDeviceID,
		RevokedDevices: make(map[string][]string, len(m.revokedDevices)),
	}

	for _, c := range m.contacts {
		state.Contacts = append(state.Contacts, models.Contact{
			ID:          c.ID,
			DisplayName: c.DisplayName,
			PublicKey:   append([]byte(nil), c.PublicKey...),
			AddedAt:     c.AddedAt,
			LastSeen:    c.LastSeen,
		})
	}

	for _, d := range m.devices {
		state.Devices = append(state.Devices, persistedDevice{
			Model:      cloneDevice(d.model),
			PrivateKey: append([]byte(nil), d.priv...),
		})
	}

	for identityID, revokedSet := range m.revokedDevices {
		ids := make([]string, 0, len(revokedSet))
		for deviceID := range revokedSet {
			ids = append(ids, deviceID)
		}
		state.RevokedDevices[identityID] = ids
	}

	raw, err := json.Marshal(state)
	if err != nil {
		return nil
	}
	return raw
}

func (m *Manager) RestoreRuntimeStateJSON(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var state persistedRuntimeState
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.contacts = make(map[string]models.Contact, len(state.Contacts))
	for _, c := range state.Contacts {
		m.contacts[c.ID] = models.Contact{
			ID:          c.ID,
			DisplayName: c.DisplayName,
			PublicKey:   append([]byte(nil), c.PublicKey...),
			AddedAt:     c.AddedAt,
			LastSeen:    c.LastSeen,
		}
	}

	m.devices = make(map[string]devicePrivate, len(state.Devices))
	for _, d := range state.Devices {
		if d.Model.ID == "" || len(d.PrivateKey) != ed25519.PrivateKeySize {
			continue
		}
		m.devices[d.Model.ID] = devicePrivate{
			model: cloneDevice(d.Model),
			priv:  append(ed25519.PrivateKey(nil), d.PrivateKey...),
		}
	}

	m.revokedDevices = make(map[string]map[string]struct{}, len(state.RevokedDevices))
	for identityID, ids := range state.RevokedDevices {
		set := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			if id == "" {
				continue
			}
			set[id] = struct{}{}
		}
		if len(set) > 0 {
			m.revokedDevices[identityID] = set
		}
	}

	if _, ok := m.devices[state.ActiveDeviceID]; ok {
		m.activeDeviceID = state.ActiveDeviceID
	} else if _, ok := m.devices[m.activeDeviceID]; !ok {
		m.activeDeviceID = ""
		for id := range m.devices {
			m.activeDeviceID = id
			break
		}
	}

	return nil
}
