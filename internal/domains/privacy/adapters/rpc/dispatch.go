package rpc

import (
	"encoding/json"
	"errors"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/internal/domains/rpckit"
)

func Dispatch(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "privacy.get":
		result, rpcErr := callWithoutParams(-32080, func() (any, error) {
			return service.GetPrivacySettings()
		})
		return result, rpcErr, true
	case "privacy.set":
		result, rpcErr := callWithSingleStringParam(rawParams, -32081, func(mode string) (any, error) {
			return service.UpdatePrivacySettings(mode)
		})
		return result, rpcErr, true
	case "privacy.storage.get":
		result, rpcErr := callWithoutParams(-32082, func() (any, error) {
			storageAPI, ok := service.(interface {
				GetStoragePolicy() (privacydomain.StoragePolicy, error)
			})
			if !ok {
				return nil, errors.New("privacy storage policy is not supported")
			}
			return storageAPI.GetStoragePolicy()
		})
		return result, rpcErr, true
	case "privacy.storage.set":
		protection, retention, messageTTL, imageTTL, fileTTL, imageQuotaMB, fileQuotaMB, imageMaxItemSizeMB, fileMaxItemSizeMB, err := decodeStoragePolicyParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32083, func() (any, error) {
			storageAPI, ok := service.(interface {
				UpdateStoragePolicy(
					storageProtection string,
					retention string,
					messageTTLSeconds int,
					imageTTLSeconds int,
					fileTTLSeconds int,
					imageQuotaMB int,
					fileQuotaMB int,
					imageMaxItemSizeMB int,
					fileMaxItemSizeMB int,
				) (privacydomain.StoragePolicy, error)
			})
			if !ok {
				return nil, errors.New("privacy storage policy is not supported")
			}
			return storageAPI.UpdateStoragePolicy(protection, retention, messageTTL, imageTTL, fileTTL, imageQuotaMB, fileQuotaMB, imageMaxItemSizeMB, fileMaxItemSizeMB)
		})
		return result, rpcErr, true
	case "privacy.storage.scope.set":
		scope, scopeID, protection, retention, messageTTL, imageTTL, fileTTL, imageQuotaMB, fileQuotaMB, imageMaxItemSizeMB, fileMaxItemSizeMB, infiniteTTL, pinRequiredForInfinite, err := decodeStorageScopePolicySetParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32084, func() (any, error) {
			storageAPI, ok := service.(interface {
				SetStorageScopeOverride(scope string, scopeID string, storageProtection string, retention string, messageTTLSeconds int, imageTTLSeconds int, fileTTLSeconds int, imageQuotaMB int, fileQuotaMB int, imageMaxItemSizeMB int, fileMaxItemSizeMB int, infiniteTTL bool, pinRequiredForInfinite bool) (privacydomain.StoragePolicyOverride, error)
			})
			if !ok {
				return nil, errors.New("privacy scoped storage policy is not supported")
			}
			return storageAPI.SetStorageScopeOverride(scope, scopeID, protection, retention, messageTTL, imageTTL, fileTTL, imageQuotaMB, fileQuotaMB, imageMaxItemSizeMB, fileMaxItemSizeMB, infiniteTTL, pinRequiredForInfinite)
		})
		return result, rpcErr, true
	case "privacy.storage.scope.get":
		scope, scopeID, _, err := decodeStorageScopePolicyRefParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32085, func() (any, error) {
			storageAPI, ok := service.(interface {
				GetStorageScopeOverride(scope string, scopeID string) (privacydomain.StoragePolicyOverride, bool, error)
			})
			if !ok {
				return nil, errors.New("privacy scoped storage policy is not supported")
			}
			override, exists, err := storageAPI.GetStorageScopeOverride(scope, scopeID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"exists": exists, "override": override}, nil
		})
		return result, rpcErr, true
	case "privacy.storage.scope.resolve":
		scope, scopeID, isPinned, err := decodeStorageScopePolicyRefParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32086, func() (any, error) {
			storageAPI, ok := service.(interface {
				ResolveStoragePolicy(scope string, scopeID string, isPinned bool) (privacydomain.StoragePolicy, error)
			})
			if !ok {
				return nil, errors.New("privacy scoped storage policy is not supported")
			}
			return storageAPI.ResolveStoragePolicy(scope, scopeID, isPinned)
		})
		return result, rpcErr, true
	case "privacy.storage.scope.delete":
		scope, scopeID, _, err := decodeStorageScopePolicyRefParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32087, func() (any, error) {
			storageAPI, ok := service.(interface {
				RemoveStorageScopeOverride(scope string, scopeID string) (bool, error)
			})
			if !ok {
				return nil, errors.New("privacy scoped storage policy is not supported")
			}
			removed, err := storageAPI.RemoveStorageScopeOverride(scope, scopeID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"removed": removed}, nil
		})
		return result, rpcErr, true
	case "blocklist.list":
		result, rpcErr := callWithoutParams(-32090, func() (any, error) {
			blocked, err := service.GetBlocklist()
			if err != nil {
				return nil, err
			}
			return map[string]any{"blocked": blocked}, nil
		})
		return result, rpcErr, true
	case "blocklist.add":
		result, rpcErr := callWithSingleStringParam(rawParams, -32091, func(identityID string) (any, error) {
			blocked, err := service.AddToBlocklist(identityID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"blocked": blocked}, nil
		})
		return result, rpcErr, true
	case "blocklist.remove":
		result, rpcErr := callWithSingleStringParam(rawParams, -32092, func(identityID string) (any, error) {
			blocked, err := service.RemoveFromBlocklist(identityID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"blocked": blocked}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func callWithoutParams(serviceErrCode int, call func() (any, error)) (any, *rpckit.Error) {
	result, err := call()
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithSingleStringParam(rawParams json.RawMessage, serviceErrCode int, call func(string) (any, error)) (any, *rpckit.Error) {
	param, err := decodeSingleStringParam(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(param)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func decodeSingleStringParam(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 && strings.TrimSpace(arr[0]) != "" {
		return arr[0], nil
	}
	return "", errors.New("invalid params")
}

func decodeSingleOrDirect[T any](raw json.RawMessage) (T, error) {
	var arr []T
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return arr[0], nil
	}
	var direct T
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	var zero T
	return zero, errors.New("invalid params")
}

func intPtrValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func decodeStoragePolicyParams(raw json.RawMessage) (string, string, int, int, int, int, int, int, int, error) {
	type payload struct {
		StorageProtectionMode string `json:"storage_protection_mode"`
		ContentRetentionMode  string `json:"content_retention_mode"`
		MessageTTLSeconds     *int   `json:"message_ttl_seconds"`
		ImageTTLSeconds       *int   `json:"image_ttl_seconds"`
		FileTTLSeconds        *int   `json:"file_ttl_seconds"`
		ImageQuotaMB          *int   `json:"image_quota_mb"`
		FileQuotaMB           *int   `json:"file_quota_mb"`
		ImageMaxItemSizeMB    *int   `json:"image_max_item_size_mb"`
		FileMaxItemSizeMB     *int   `json:"file_max_item_size_mb"`
	}

	decodePayload := func(p payload) (string, string, int, int, int, int, int, int, int, error) {
		protection := strings.TrimSpace(p.StorageProtectionMode)
		retention := strings.TrimSpace(p.ContentRetentionMode)
		if protection == "" || retention == "" {
			return "", "", 0, 0, 0, 0, 0, 0, 0, errors.New("invalid params")
		}
		return protection, retention,
			intPtrValue(p.MessageTTLSeconds),
			intPtrValue(p.ImageTTLSeconds),
			intPtrValue(p.FileTTLSeconds),
			intPtrValue(p.ImageQuotaMB),
			intPtrValue(p.FileQuotaMB),
			intPtrValue(p.ImageMaxItemSizeMB),
			intPtrValue(p.FileMaxItemSizeMB),
			nil
	}

	p, err := decodeSingleOrDirect[payload](raw)
	if err == nil {
		return decodePayload(p)
	}
	return "", "", 0, 0, 0, 0, 0, 0, 0, errors.New("invalid params")
}

func decodeStorageScopePolicySetParams(raw json.RawMessage) (string, string, string, string, int, int, int, int, int, int, int, bool, bool, error) {
	type payload struct {
		Scope                  string `json:"scope"`
		ScopeID                string `json:"scope_id"`
		StorageProtectionMode  string `json:"storage_protection_mode"`
		ContentRetentionMode   string `json:"content_retention_mode"`
		MessageTTLSeconds      *int   `json:"message_ttl_seconds"`
		ImageTTLSeconds        *int   `json:"image_ttl_seconds"`
		FileTTLSeconds         *int   `json:"file_ttl_seconds"`
		ImageQuotaMB           *int   `json:"image_quota_mb"`
		FileQuotaMB            *int   `json:"file_quota_mb"`
		ImageMaxItemSizeMB     *int   `json:"image_max_item_size_mb"`
		FileMaxItemSizeMB      *int   `json:"file_max_item_size_mb"`
		InfiniteTTL            bool   `json:"infinite_ttl"`
		PinRequiredForInfinite bool   `json:"pin_required_for_infinite"`
	}
	parse := func(p payload) (string, string, string, string, int, int, int, int, int, int, int, bool, bool, error) {
		scope := strings.TrimSpace(p.Scope)
		scopeID := strings.TrimSpace(p.ScopeID)
		protection := strings.TrimSpace(p.StorageProtectionMode)
		retention := strings.TrimSpace(p.ContentRetentionMode)
		if scope == "" || protection == "" || retention == "" {
			return "", "", "", "", 0, 0, 0, 0, 0, 0, 0, false, false, errors.New("invalid params")
		}
		return scope, scopeID, protection, retention,
			intPtrValue(p.MessageTTLSeconds),
			intPtrValue(p.ImageTTLSeconds),
			intPtrValue(p.FileTTLSeconds),
			intPtrValue(p.ImageQuotaMB),
			intPtrValue(p.FileQuotaMB),
			intPtrValue(p.ImageMaxItemSizeMB),
			intPtrValue(p.FileMaxItemSizeMB),
			p.InfiniteTTL,
			p.PinRequiredForInfinite,
			nil
	}
	p, err := decodeSingleOrDirect[payload](raw)
	if err == nil {
		return parse(p)
	}
	return "", "", "", "", 0, 0, 0, 0, 0, 0, 0, false, false, errors.New("invalid params")
}

func decodeStorageScopePolicyRefParams(raw json.RawMessage) (string, string, bool, error) {
	type payload struct {
		Scope    string `json:"scope"`
		ScopeID  string `json:"scope_id"`
		IsPinned bool   `json:"is_pinned"`
	}
	parse := func(p payload) (string, string, bool, error) {
		scope := strings.TrimSpace(p.Scope)
		scopeID := strings.TrimSpace(p.ScopeID)
		if scope == "" {
			return "", "", false, errors.New("invalid params")
		}
		return scope, scopeID, p.IsPinned, nil
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return parse(arr[0])
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parse(direct)
	}
	return "", "", false, errors.New("invalid params")
}
