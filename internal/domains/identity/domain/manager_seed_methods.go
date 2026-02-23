package domain

import "encoding/json"

func (m *Manager) SnapshotSeedEnvelope() *EncryptedSeedEnvelope {
	return m.seeds.SnapshotEnvelope()
}

func (m *Manager) RestoreSeedEnvelope(env *EncryptedSeedEnvelope) {
	m.seeds.RestoreEnvelope(env)
}

func (m *Manager) SnapshotSeedEnvelopeJSON() []byte {
	env := m.seeds.SnapshotEnvelope()
	if env == nil {
		return nil
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	return raw
}

func (m *Manager) RestoreSeedEnvelopeJSON(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var env EncryptedSeedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	m.seeds.RestoreEnvelope(&env)
	return nil
}
