package rpc

import (
	"encoding/json"
	"errors"

	identityusecase "aim-chat/go-backend/internal/domains/identity/usecase"
	"aim-chat/go-backend/pkg/models"
)

func (s *Server) dispatchContactFileRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	if result, rpcErr, ok := s.dispatchContactRPC(method, rawParams); ok {
		return result, rpcErr, true
	}
	if result, rpcErr, ok := s.dispatchFileUploadRPC(method, rawParams); ok {
		return result, rpcErr, true
	}
	if result, rpcErr, ok := s.dispatchBlobRPC(method, rawParams); ok {
		return result, rpcErr, true
	}
	if result, rpcErr, ok := s.dispatchNodeBindingRPC(method, rawParams); ok {
		return result, rpcErr, true
	}
	return nil, nil, false
}

func (s *Server) dispatchContactRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "contact.verify":
		result, rpcErr := callWithCardParam(rawParams, -32010, func(card models.ContactCard) (any, error) {
			ok, err := s.service.VerifyContactCard(card)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"valid": ok}, nil
		})
		return result, rpcErr, true
	case "contact.add":
		result, rpcErr := s.dispatchContactAdd(rawParams)
		return result, rpcErr, true
	case "contact.add_by_id":
		result, rpcErr := callWithContactByIDParams(rawParams, func(contactID, displayName string) (any, *rpcError) {
			return s.dispatchAddContactByID(contactID, displayName)
		})
		return result, rpcErr, true
	case "contact.list":
		result, rpcErr := callWithoutParams(-32012, func() (any, error) {
			return s.service.GetContacts()
		})
		return result, rpcErr, true
	case "contact.remove":
		result, rpcErr := callWithSingleStringParam(rawParams, -32014, func(contactID string) (any, error) {
			if err := s.service.RemoveContact(contactID); err != nil {
				return nil, err
			}
			return map[string]bool{"removed": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchFileUploadRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "file.put":
		result, rpcErr := callWithFilePutParams(rawParams, -32060, func(name, mimeType, dataBase64 string) (any, error) {
			return s.service.PutAttachment(name, mimeType, dataBase64)
		})
		return result, rpcErr, true
	case "file.upload.init":
		return s.dispatchFileUploadInit(rawParams)
	case "file.upload.chunk":
		return s.dispatchFileUploadChunk(rawParams)
	case "file.upload.status":
		return s.dispatchFileUploadStatus(rawParams)
	case "file.upload.commit":
		return s.dispatchFileUploadCommit(rawParams)
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchFileUploadInit(rawParams json.RawMessage) (any, *rpcError, bool) {
	params, err := decodeFileUploadInitParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32061, func() (any, error) {
		uploader, ok := s.service.(interface {
			InitAttachmentUpload(name, mimeType string, totalSize int64, totalChunks, chunkSize int, fileSHA256 string) (identityusecase.AttachmentUploadInitResult, error)
		})
		if !ok {
			return nil, errors.New("chunked file upload is not supported")
		}
		return uploader.InitAttachmentUpload(params.Name, params.MimeType, params.TotalSize, params.TotalChunks, params.ChunkSize, params.FileSHA256)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchFileUploadChunk(rawParams json.RawMessage) (any, *rpcError, bool) {
	params, err := decodeFileUploadChunkParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32062, func() (any, error) {
		uploader, ok := s.service.(interface {
			PutAttachmentChunk(uploadID string, chunkIndex int, dataBase64, chunkSHA256 string) (identityusecase.AttachmentUploadChunkResult, error)
		})
		if !ok {
			return nil, errors.New("chunked file upload is not supported")
		}
		return uploader.PutAttachmentChunk(params.UploadID, params.ChunkIndex, params.DataBase64, params.ChunkSHA256)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchFileUploadStatus(rawParams json.RawMessage) (any, *rpcError, bool) {
	params, err := decodeFileUploadCommitParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32063, func() (any, error) {
		uploader, ok := s.service.(interface {
			GetAttachmentUploadStatus(uploadID string) (identityusecase.AttachmentUploadStatus, error)
		})
		if !ok {
			return nil, errors.New("chunked file upload is not supported")
		}
		return uploader.GetAttachmentUploadStatus(params.UploadID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchFileUploadCommit(rawParams json.RawMessage) (any, *rpcError, bool) {
	params, err := decodeFileUploadCommitParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32064, func() (any, error) {
		uploader, ok := s.service.(interface {
			CommitAttachmentUpload(uploadID string) (models.AttachmentMeta, error)
		})
		if !ok {
			return nil, errors.New("chunked file upload is not supported")
		}
		return uploader.CommitAttachmentUpload(params.UploadID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "blob.providers.list":
		return s.dispatchBlobProvidersList(rawParams)
	case "blob.pin":
		return s.dispatchBlobPin(rawParams)
	case "blob.unpin":
		return s.dispatchBlobUnpin(rawParams)
	case "blob.replication.get":
		return s.dispatchBlobReplicationGet()
	case "blob.replication.set":
		return s.dispatchBlobReplicationSet(rawParams)
	case "blob.features.get":
		return s.dispatchBlobFeaturesGet()
	case "blob.features.set":
		return s.dispatchBlobFeaturesSet(rawParams)
	case "blob.acl.get":
		return s.dispatchBlobACLGet()
	case "blob.acl.set":
		return s.dispatchBlobACLSet(rawParams)
	case "blob.preset.get":
		return s.dispatchBlobPresetGet()
	case "blob.preset.set":
		return s.dispatchBlobPresetSet(rawParams)
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchBlobProvidersList(rawParams json.RawMessage) (any, *rpcError, bool) {
	blobID, err := decodeBlobProvidersParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32065, func() (any, error) {
		return s.service.ListBlobProviders(blobID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobPin(rawParams json.RawMessage) (any, *rpcError, bool) {
	blobID, err := decodeBlobProvidersParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32066, func() (any, error) {
		pinner, ok := s.service.(interface {
			PinBlob(blobID string) (models.AttachmentMeta, error)
		})
		if !ok {
			return nil, errors.New("blob pinning is not supported")
		}
		return pinner.PinBlob(blobID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobUnpin(rawParams json.RawMessage) (any, *rpcError, bool) {
	blobID, err := decodeBlobProvidersParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32067, func() (any, error) {
		pinner, ok := s.service.(interface {
			UnpinBlob(blobID string) (models.AttachmentMeta, error)
		})
		if !ok {
			return nil, errors.New("blob pinning is not supported")
		}
		return pinner.UnpinBlob(blobID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobReplicationGet() (any, *rpcError, bool) {
	result, rpcErr := callWithoutParams(-32068, func() (any, error) {
		replicationAPI, ok := s.service.(interface {
			GetBlobReplicationMode() string
		})
		if !ok {
			return nil, errors.New("blob replication mode is not supported")
		}
		return map[string]string{"mode": replicationAPI.GetBlobReplicationMode()}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobReplicationSet(rawParams json.RawMessage) (any, *rpcError, bool) {
	mode, err := decodeBlobReplicationModeParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32069, func() (any, error) {
		replicationAPI, ok := s.service.(interface {
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
}

func (s *Server) dispatchBlobFeaturesGet() (any, *rpcError, bool) {
	result, rpcErr := callWithoutParams(-32071, func() (any, error) {
		featureAPI, ok := s.service.(interface {
			GetBlobFeatureFlags() models.BlobFeatureFlags
		})
		if !ok {
			return nil, errors.New("blob feature flags are not supported")
		}
		return featureAPI.GetBlobFeatureFlags(), nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobFeaturesSet(rawParams json.RawMessage) (any, *rpcError, bool) {
	announceEnabled, fetchEnabled, rolloutPercent, err := decodeBlobFeatureFlagsParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32072, func() (any, error) {
		featureAPI, ok := s.service.(interface {
			SetBlobFeatureFlags(announceEnabled, fetchEnabled bool, rolloutPercent int) (models.BlobFeatureFlags, error)
		})
		if !ok {
			return nil, errors.New("blob feature flags are not supported")
		}
		return featureAPI.SetBlobFeatureFlags(announceEnabled, fetchEnabled, rolloutPercent)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobACLGet() (any, *rpcError, bool) {
	result, rpcErr := callWithoutParams(-32077, func() (any, error) {
		aclAPI, ok := s.service.(interface {
			GetBlobACLPolicy() models.BlobACLPolicy
		})
		if !ok {
			return nil, errors.New("blob acl policy is not supported")
		}
		return aclAPI.GetBlobACLPolicy(), nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobACLSet(rawParams json.RawMessage) (any, *rpcError, bool) {
	mode, allowlist, err := decodeBlobACLPolicyParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32078, func() (any, error) {
		aclAPI, ok := s.service.(interface {
			SetBlobACLPolicy(mode string, allowlist []string) (models.BlobACLPolicy, error)
		})
		if !ok {
			return nil, errors.New("blob acl policy is not supported")
		}
		return aclAPI.SetBlobACLPolicy(mode, allowlist)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobPresetGet() (any, *rpcError, bool) {
	result, rpcErr := callWithoutParams(-32079, func() (any, error) {
		presetAPI, ok := s.service.(interface {
			GetBlobNodePreset() models.BlobNodePresetConfig
		})
		if !ok {
			return nil, errors.New("blob node preset is not supported")
		}
		return presetAPI.GetBlobNodePreset(), nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchBlobPresetSet(rawParams json.RawMessage) (any, *rpcError, bool) {
	preset, err := decodeBlobNodePresetParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32088, func() (any, error) {
		presetAPI, ok := s.service.(interface {
			SetBlobNodePreset(preset string) (models.BlobNodePresetConfig, error)
		})
		if !ok {
			return nil, errors.New("blob node preset is not supported")
		}
		return presetAPI.SetBlobNodePreset(preset)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchNodeBindingRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "node.binding.link.create":
		return s.dispatchNodeBindingLinkCreate(rawParams)
	case "node.binding.complete":
		return s.dispatchNodeBindingComplete(rawParams)
	case "node.binding.get":
		return s.dispatchNodeBindingGet()
	case "node.binding.unbind":
		return s.dispatchNodeBindingUnbind(rawParams)
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchNodeBindingLinkCreate(rawParams json.RawMessage) (any, *rpcError, bool) {
	ttlSeconds, err := decodeNodeBindingLinkCreateParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32073, func() (any, error) {
		bindingAPI, ok := s.service.(interface {
			CreateNodeBindingLinkCode(ttlSeconds int) (models.NodeBindingLinkCode, error)
		})
		if !ok {
			return nil, errors.New("node binding is not supported")
		}
		return bindingAPI.CreateNodeBindingLinkCode(ttlSeconds)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchNodeBindingComplete(rawParams json.RawMessage) (any, *rpcError, bool) {
	linkCode, nodeID, nodePub, nodeSig, rebind, err := decodeNodeBindingCompleteParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32074, func() (any, error) {
		bindingAPI, ok := s.service.(interface {
			CompleteNodeBinding(linkCode, nodeID, nodePublicKeyBase64, nodeSignatureBase64 string, allowRebind bool) (models.NodeBindingRecord, error)
		})
		if !ok {
			return nil, errors.New("node binding is not supported")
		}
		return bindingAPI.CompleteNodeBinding(linkCode, nodeID, nodePub, nodeSig, rebind)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchNodeBindingGet() (any, *rpcError, bool) {
	result, rpcErr := callWithoutParams(-32075, func() (any, error) {
		bindingAPI, ok := s.service.(interface {
			GetNodeBinding() (models.NodeBindingRecord, bool, error)
		})
		if !ok {
			return nil, errors.New("node binding is not supported")
		}
		record, exists, err := bindingAPI.GetNodeBinding()
		if err != nil {
			return nil, err
		}
		return map[string]any{"exists": exists, "binding": record}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchNodeBindingUnbind(rawParams json.RawMessage) (any, *rpcError, bool) {
	nodeID, confirm, err := decodeNodeBindingUnbindParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams(), true
	}
	result, rpcErr := callWithoutParams(-32076, func() (any, error) {
		bindingAPI, ok := s.service.(interface {
			UnbindNode(nodeID string, confirm bool) (bool, error)
		})
		if !ok {
			return nil, errors.New("node binding is not supported")
		}
		removed, err := bindingAPI.UnbindNode(nodeID, confirm)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"removed": removed}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchContactAdd(rawParams json.RawMessage) (any, *rpcError) {
	card, contactID, displayName, err := decodeAddContactParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	if card.IdentityID != "" {
		if err := s.service.AddContactCard(card); err != nil {
			return nil, rpcServiceError(-32011, err)
		}
		return map[string]bool{"added": true}, nil
	}
	return s.dispatchAddContactByID(contactID, displayName)
}

func (s *Server) dispatchAddContactByID(contactID, displayName string) (any, *rpcError) {
	if err := s.service.AddContact(contactID, displayName); err != nil {
		return nil, rpcServiceError(-32013, err)
	}
	return map[string]bool{"added": true}, nil
}
