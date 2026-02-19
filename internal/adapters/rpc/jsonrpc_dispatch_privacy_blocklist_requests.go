package rpc

import (
	"encoding/json"
	"errors"

	privacydomain "aim-chat/go-backend/internal/domains/privacy"
)

func (s *Server) dispatchPrivacyRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "privacy.get":
		result, rpcErr := callWithoutParams(-32080, func() (any, error) {
			return s.service.GetPrivacySettings()
		})
		return result, rpcErr, true
	case "privacy.set":
		result, rpcErr := callWithSingleStringParam(rawParams, -32081, func(mode string) (any, error) {
			return s.service.UpdatePrivacySettings(mode)
		})
		return result, rpcErr, true
	case "privacy.storage.get":
		return s.dispatchPrivacyStorageGet()
	case "privacy.storage.set":
		return s.dispatchPrivacyStorageSet(rawParams)
	default:
		return s.dispatchPrivacyStorageScopeRPC(method, rawParams)
	}
}

func (s *Server) dispatchPrivacyStorageGet() (any, *rpcError, bool) {
	result, rpcErr := callWithoutParams(-32082, func() (any, error) {
		storageAPI, ok := s.service.(interface {
			GetStoragePolicy() (privacydomain.StoragePolicy, error)
		})
		if !ok {
			return nil, errors.New("privacy storage policy is not supported")
		}
		return storageAPI.GetStoragePolicy()
	})
	return result, rpcErr, true
}

func (s *Server) dispatchPrivacyStorageSet(rawParams json.RawMessage) (any, *rpcError, bool) {
	protection, retention, messageTTL, imageTTL, fileTTL, imageQuotaMB, fileQuotaMB, imageMaxItemSizeMB, fileMaxItemSizeMB, err := decodeStoragePolicyParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32083, func() (any, error) {
		storageAPI, ok := s.service.(interface {
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
}

func (s *Server) dispatchPrivacyStorageScopeRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "privacy.storage.scope.set":
		return s.dispatchPrivacyStorageScopeSet(rawParams)
	case "privacy.storage.scope.get":
		return s.dispatchPrivacyStorageScopeGet(rawParams)
	case "privacy.storage.scope.resolve":
		return s.dispatchPrivacyStorageScopeResolve(rawParams)
	case "privacy.storage.scope.delete":
		return s.dispatchPrivacyStorageScopeDelete(rawParams)
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchPrivacyStorageScopeSet(rawParams json.RawMessage) (any, *rpcError, bool) {
	scope, scopeID, protection, retention, messageTTL, imageTTL, fileTTL, imageQuotaMB, fileQuotaMB, imageMaxItemSizeMB, fileMaxItemSizeMB, infiniteTTL, pinRequiredForInfinite, err := decodeStorageScopePolicySetParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32084, func() (any, error) {
		storageAPI, ok := s.service.(interface {
			SetStorageScopeOverride(scope string, scopeID string, storageProtection string, retention string, messageTTLSeconds int, imageTTLSeconds int, fileTTLSeconds int, imageQuotaMB int, fileQuotaMB int, imageMaxItemSizeMB int, fileMaxItemSizeMB int, infiniteTTL bool, pinRequiredForInfinite bool) (privacydomain.StoragePolicyOverride, error)
		})
		if !ok {
			return nil, errors.New("privacy scoped storage policy is not supported")
		}
		return storageAPI.SetStorageScopeOverride(scope, scopeID, protection, retention, messageTTL, imageTTL, fileTTL, imageQuotaMB, fileQuotaMB, imageMaxItemSizeMB, fileMaxItemSizeMB, infiniteTTL, pinRequiredForInfinite)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchPrivacyStorageScopeGet(rawParams json.RawMessage) (any, *rpcError, bool) {
	scope, scopeID, _, err := decodeStorageScopePolicyRefParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32085, func() (any, error) {
		storageAPI, ok := s.service.(interface {
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
}

func (s *Server) dispatchPrivacyStorageScopeResolve(rawParams json.RawMessage) (any, *rpcError, bool) {
	scope, scopeID, isPinned, err := decodeStorageScopePolicyRefParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32086, func() (any, error) {
		storageAPI, ok := s.service.(interface {
			ResolveStoragePolicy(scope string, scopeID string, isPinned bool) (privacydomain.StoragePolicy, error)
		})
		if !ok {
			return nil, errors.New("privacy scoped storage policy is not supported")
		}
		return storageAPI.ResolveStoragePolicy(scope, scopeID, isPinned)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchPrivacyStorageScopeDelete(rawParams json.RawMessage) (any, *rpcError, bool) {
	scope, scopeID, _, err := decodeStorageScopePolicyRefParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32087, func() (any, error) {
		storageAPI, ok := s.service.(interface {
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
}

func (s *Server) dispatchBlocklistRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "blocklist.list":
		result, rpcErr := callWithoutParams(-32090, func() (any, error) {
			blocked, err := s.service.GetBlocklist()
			if err != nil {
				return nil, err
			}
			return map[string]any{"blocked": blocked}, nil
		})
		return result, rpcErr, true
	case "blocklist.add":
		result, rpcErr := callWithSingleStringParam(rawParams, -32091, func(identityID string) (any, error) {
			blocked, err := s.service.AddToBlocklist(identityID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"blocked": blocked}, nil
		})
		return result, rpcErr, true
	case "blocklist.remove":
		result, rpcErr := callWithSingleStringParam(rawParams, -32092, func(identityID string) (any, error) {
			blocked, err := s.service.RemoveFromBlocklist(identityID)
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

func (s *Server) dispatchRequestRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "request.list":
		result, rpcErr := callWithoutParams(-32093, func() (any, error) {
			return s.service.ListMessageRequests()
		})
		return result, rpcErr, true
	case "request.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32094, func(senderID string) (any, error) {
			return s.service.GetMessageRequest(senderID)
		})
		return result, rpcErr, true
	case "request.accept":
		result, rpcErr := callWithSingleStringParam(rawParams, -32095, func(senderID string) (any, error) {
			accepted, err := s.service.AcceptMessageRequest(senderID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"accepted": accepted}, nil
		})
		return result, rpcErr, true
	case "request.decline":
		result, rpcErr := callWithSingleStringParam(rawParams, -32096, func(senderID string) (any, error) {
			declined, err := s.service.DeclineMessageRequest(senderID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"declined": declined}, nil
		})
		return result, rpcErr, true
	case "request.block":
		result, rpcErr := callWithSingleStringParam(rawParams, -32097, func(senderID string) (any, error) {
			return s.service.BlockSender(senderID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}
