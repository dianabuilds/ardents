package daemonservice

import (
	"errors"

	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/pkg/models"
)

var errInvalidNodePoliciesPatch = errors.New("invalid node policies patch")

func (s *Service) GetNodePolicies() models.NodePolicies {
	policies, err := s.privacyCore.GetNodePolicies()
	if err != nil {
		return nodePoliciesFromPreset(s.GetBlobNodePreset())
	}
	return nodePoliciesToModels(policies)
}

func (s *Service) UpdateNodePolicies(patch models.NodePoliciesPatch) (models.NodePolicies, error) {
	if patch.Personal == nil && patch.Public == nil {
		return models.NodePolicies{}, errInvalidNodePoliciesPatch
	}
	current, err := s.privacyCore.GetNodePolicies()
	if err != nil {
		return models.NodePolicies{}, err
	}
	next := applyNodePoliciesPatch(current, patch)
	updated, err := s.privacyCore.UpdateNodePolicies(next)
	if err != nil {
		return models.NodePolicies{}, err
	}
	s.applyNodePolicies(updated)
	return nodePoliciesToModels(updated), nil
}

func (s *Service) applyNodePoliciesFromSettings(settings privacydomain.PrivacySettings) {
	normalized := privacydomain.NormalizePrivacySettings(settings)
	if normalized.NodePolicies == nil {
		return
	}
	s.applyNodePolicies(*normalized.NodePolicies)
}

func (s *Service) applyNodePolicies(policies privacydomain.NodePolicies) {
	s.presetMu.Lock()
	cfg := s.nodePreset
	cfg.PersonalStoreEnabled = policies.Personal.StoreEnabled
	cfg.PublicStoreEnabled = policies.Public.StoreEnabled
	cfg.RelayEnabled = policies.Public.RelayEnabled
	cfg.PublicDiscoveryEnabled = policies.Public.DiscoveryEnabled
	cfg.PublicServingEnabled = policies.Public.ServingEnabled
	s.nodePreset = cfg
	s.presetMu.Unlock()
}

func (s *Service) syncNodePoliciesFromPreset(cfg blobNodePresetConfig) error {
	policies := privacydomain.DefaultNodePolicies()
	policies.Personal.StoreEnabled = cfg.PersonalStoreEnabled
	policies.Public.StoreEnabled = cfg.PublicStoreEnabled
	policies.Public.RelayEnabled = cfg.RelayEnabled
	policies.Public.DiscoveryEnabled = cfg.PublicDiscoveryEnabled
	policies.Public.ServingEnabled = cfg.PublicServingEnabled
	_, err := s.privacyCore.UpdateNodePolicies(policies)
	return err
}

func nodePoliciesToModels(in privacydomain.NodePolicies) models.NodePolicies {
	return models.NodePolicies{
		ProfileSchemaVersion: in.ProfileSchemaVersion,
		Personal: models.NodePersonalPolicy{
			StoreEnabled: in.Personal.StoreEnabled,
			TTLDays:      in.Personal.TTLDays,
			QuotaMB:      in.Personal.QuotaMB,
			PinEnabled:   in.Personal.PinEnabled,
		},
		Public: models.NodePublicPolicy{
			RelayEnabled:     in.Public.RelayEnabled,
			DiscoveryEnabled: in.Public.DiscoveryEnabled,
			ServingEnabled:   in.Public.ServingEnabled,
			StoreEnabled:     in.Public.StoreEnabled,
			TTLDays:          in.Public.TTLDays,
			QuotaMB:          in.Public.QuotaMB,
		},
	}
}

func nodePoliciesFromPreset(preset models.BlobNodePresetConfig) models.NodePolicies {
	return models.NodePolicies{
		ProfileSchemaVersion: privacydomain.CurrentProfileSchemaVersion,
		Personal: models.NodePersonalPolicy{
			StoreEnabled: preset.PersonalStoreEnabled,
		},
		Public: models.NodePublicPolicy{
			RelayEnabled:     preset.RelayEnabled,
			DiscoveryEnabled: preset.PublicDiscoveryEnabled,
			ServingEnabled:   preset.PublicServingEnabled,
			StoreEnabled:     preset.PublicStoreEnabled,
		},
	}
}

func applyNodePoliciesPatch(current privacydomain.NodePolicies, patch models.NodePoliciesPatch) privacydomain.NodePolicies {
	next := current
	if patch.Personal != nil {
		if patch.Personal.StoreEnabled != nil {
			next.Personal.StoreEnabled = *patch.Personal.StoreEnabled
		}
		if patch.Personal.PinEnabled != nil {
			next.Personal.PinEnabled = *patch.Personal.PinEnabled
		}
		if patch.Personal.TTLDays != nil {
			next.Personal.TTLDays = *patch.Personal.TTLDays
		}
		if patch.Personal.QuotaMB != nil {
			next.Personal.QuotaMB = *patch.Personal.QuotaMB
		}
	}
	if patch.Public != nil {
		if patch.Public.RelayEnabled != nil {
			next.Public.RelayEnabled = *patch.Public.RelayEnabled
		}
		if patch.Public.DiscoveryEnabled != nil {
			next.Public.DiscoveryEnabled = *patch.Public.DiscoveryEnabled
		}
		if patch.Public.ServingEnabled != nil {
			next.Public.ServingEnabled = *patch.Public.ServingEnabled
		}
		if patch.Public.StoreEnabled != nil {
			next.Public.StoreEnabled = *patch.Public.StoreEnabled
		}
		if patch.Public.TTLDays != nil {
			next.Public.TTLDays = *patch.Public.TTLDays
		}
		if patch.Public.QuotaMB != nil {
			next.Public.QuotaMB = *patch.Public.QuotaMB
		}
	}
	normalized := privacydomain.NormalizePrivacySettings(privacydomain.PrivacySettings{
		NodePolicies: &next,
	})
	if normalized.NodePolicies == nil {
		return privacydomain.DefaultNodePolicies()
	}
	return *normalized.NodePolicies
}
