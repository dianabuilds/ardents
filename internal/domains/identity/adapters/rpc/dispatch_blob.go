package rpc

import (
	"encoding/json"
	"errors"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/domains/rpckit"
	"aim-chat/go-backend/pkg/models"
)

func dispatchBlobRPC(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "blob.providers.list":
		blobID, err := decodeBlobProvidersParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32065, func() (any, error) {
			return service.ListBlobProviders(blobID)
		})
		return result, rpcErr, true
	case "blob.pin":
		blobID, err := decodeBlobProvidersParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32066, func() (any, error) {
			pinner, ok := service.(interface {
				PinBlob(blobID string) (models.AttachmentMeta, error)
			})
			if !ok {
				return nil, errors.New("blob pinning is not supported")
			}
			return pinner.PinBlob(blobID)
		})
		return result, rpcErr, true
	case "blob.unpin":
		blobID, err := decodeBlobProvidersParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32067, func() (any, error) {
			pinner, ok := service.(interface {
				UnpinBlob(blobID string) (models.AttachmentMeta, error)
			})
			if !ok {
				return nil, errors.New("blob pinning is not supported")
			}
			return pinner.UnpinBlob(blobID)
		})
		return result, rpcErr, true
	case "blob.replication.get":
		result, rpcErr := callWithoutParams(-32068, func() (any, error) {
			replicationAPI, ok := service.(interface {
				GetBlobReplicationMode() string
			})
			if !ok {
				return nil, errors.New("blob replication mode is not supported")
			}
			return map[string]string{"mode": replicationAPI.GetBlobReplicationMode()}, nil
		})
		return result, rpcErr, true
	case "blob.replication.set":
		mode, err := decodeBlobReplicationModeParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32069, func() (any, error) {
			replicationAPI, ok := service.(interface {
				SetBlobReplicationMode(mode string) error
				GetBlobReplicationMode() string
			})
			if !ok {
				return nil, errors.New("blob replication mode is not supported")
			}
			if err := replicationAPI.SetBlobReplicationMode(mode); err != nil {
				return nil, err
			}
			return map[string]string{"mode": replicationAPI.GetBlobReplicationMode()}, nil
		})
		return result, rpcErr, true
	case "blob.features.get":
		result, rpcErr := callWithoutParams(-32071, func() (any, error) {
			featureAPI, ok := service.(interface {
				GetBlobFeatureFlags() models.BlobFeatureFlags
			})
			if !ok {
				return nil, errors.New("blob feature flags are not supported")
			}
			return featureAPI.GetBlobFeatureFlags(), nil
		})
		return result, rpcErr, true
	case "blob.features.set":
		announceEnabled, fetchEnabled, rolloutPercent, err := decodeBlobFeatureFlagsParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32072, func() (any, error) {
			featureAPI, ok := service.(interface {
				SetBlobFeatureFlags(announceEnabled, fetchEnabled bool, rolloutPercent int) (models.BlobFeatureFlags, error)
			})
			if !ok {
				return nil, errors.New("blob feature flags are not supported")
			}
			return featureAPI.SetBlobFeatureFlags(announceEnabled, fetchEnabled, rolloutPercent)
		})
		return result, rpcErr, true
	case "blob.acl.get":
		result, rpcErr := callWithoutParams(-32077, func() (any, error) {
			aclAPI, ok := service.(interface {
				GetBlobACLPolicy() models.BlobACLPolicy
			})
			if !ok {
				return nil, errors.New("blob acl policy is not supported")
			}
			return aclAPI.GetBlobACLPolicy(), nil
		})
		return result, rpcErr, true
	case "blob.acl.set":
		mode, allowlist, err := decodeBlobACLPolicyParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32078, func() (any, error) {
			aclAPI, ok := service.(interface {
				SetBlobACLPolicy(mode string, allowlist []string) (models.BlobACLPolicy, error)
			})
			if !ok {
				return nil, errors.New("blob acl policy is not supported")
			}
			return aclAPI.SetBlobACLPolicy(mode, allowlist)
		})
		return result, rpcErr, true
	case "blob.preset.get":
		result, rpcErr := callWithoutParams(-32079, func() (any, error) {
			presetAPI, ok := service.(interface {
				GetBlobNodePreset() models.BlobNodePresetConfig
			})
			if !ok {
				return nil, errors.New("blob node preset is not supported")
			}
			return presetAPI.GetBlobNodePreset(), nil
		})
		return result, rpcErr, true
	case "blob.preset.set":
		preset, err := decodeBlobNodePresetParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32088, func() (any, error) {
			presetAPI, ok := service.(interface {
				SetBlobNodePreset(preset string) (models.BlobNodePresetConfig, error)
			})
			if !ok {
				return nil, errors.New("blob node preset is not supported")
			}
			return presetAPI.SetBlobNodePreset(preset)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func decodeBlobProvidersParams(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		blobID := strings.TrimSpace(arr[0])
		if blobID == "" {
			return "", errors.New("invalid params")
		}
		return blobID, nil
	}
	var payload struct {
		BlobID string `json:"blob_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", errors.New("invalid params")
	}
	blobID := strings.TrimSpace(payload.BlobID)
	if blobID == "" {
		return "", errors.New("invalid params")
	}
	return blobID, nil
}

func decodeBlobReplicationModeParams(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		mode := strings.TrimSpace(arr[0])
		if mode == "" {
			return "", errors.New("invalid params")
		}
		return mode, nil
	}
	var payload struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", errors.New("invalid params")
	}
	mode := strings.TrimSpace(payload.Mode)
	if mode == "" {
		return "", errors.New("invalid params")
	}
	return mode, nil
}

func decodeBlobFeatureFlagsParams(raw json.RawMessage) (bool, bool, int, error) {
	type payload struct {
		AnnounceEnabled *bool `json:"announce_enabled"`
		FetchEnabled    *bool `json:"fetch_enabled"`
		RolloutPercent  *int  `json:"rollout_percent"`
	}
	parse := func(p payload) (bool, bool, int, error) {
		if p.AnnounceEnabled == nil || p.FetchEnabled == nil || p.RolloutPercent == nil {
			return false, false, 0, errors.New("invalid params")
		}
		if *p.RolloutPercent < 0 || *p.RolloutPercent > 100 {
			return false, false, 0, errors.New("invalid params")
		}
		return *p.AnnounceEnabled, *p.FetchEnabled, *p.RolloutPercent, nil
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return parse(arr[0])
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parse(direct)
	}
	return false, false, 0, errors.New("invalid params")
}

func decodeBlobACLPolicyParams(raw json.RawMessage) (string, []string, error) {
	type payload struct {
		Mode      string   `json:"mode"`
		Allowlist []string `json:"allowlist"`
	}
	parse := func(p payload) (string, []string, error) {
		mode := strings.TrimSpace(p.Mode)
		if mode == "" {
			return "", nil, errors.New("invalid params")
		}
		return mode, p.Allowlist, nil
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return parse(arr[0])
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parse(direct)
	}
	return "", nil, errors.New("invalid params")
}

func decodeBlobNodePresetParams(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		preset := strings.TrimSpace(arr[0])
		if preset == "" {
			return "", errors.New("invalid params")
		}
		return preset, nil
	}
	var payload struct {
		Preset string `json:"preset"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", errors.New("invalid params")
	}
	preset := strings.TrimSpace(payload.Preset)
	if preset == "" {
		return "", errors.New("invalid params")
	}
	return preset, nil
}
