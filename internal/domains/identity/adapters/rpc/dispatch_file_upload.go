package rpc

import (
	"encoding/json"
	"errors"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	identityusecase "aim-chat/go-backend/internal/domains/identity/usecase"
	"aim-chat/go-backend/internal/domains/rpckit"
	"aim-chat/go-backend/pkg/models"
)

func dispatchFileUploadRPC(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "file.put":
		result, rpcErr := callWithFilePutParams(rawParams, -32060, func(name, mimeType, dataBase64 string) (any, error) {
			return service.PutAttachment(name, mimeType, dataBase64)
		})
		return result, rpcErr, true
	case "file.upload.init":
		params, err := decodeFileUploadInitParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32061, func() (any, error) {
			uploader, ok := service.(interface {
				InitAttachmentUpload(name, mimeType string, totalSize int64, totalChunks, chunkSize int, fileSHA256 string) (identityusecase.AttachmentUploadInitResult, error)
			})
			if !ok {
				return nil, errors.New("chunked file upload is not supported")
			}
			return uploader.InitAttachmentUpload(params.Name, params.MimeType, params.TotalSize, params.TotalChunks, params.ChunkSize, params.FileSHA256)
		})
		return result, rpcErr, true
	case "file.upload.chunk":
		params, err := decodeFileUploadChunkParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32062, func() (any, error) {
			uploader, ok := service.(interface {
				PutAttachmentChunk(uploadID string, chunkIndex int, dataBase64, chunkSHA256 string) (identityusecase.AttachmentUploadChunkResult, error)
			})
			if !ok {
				return nil, errors.New("chunked file upload is not supported")
			}
			return uploader.PutAttachmentChunk(params.UploadID, params.ChunkIndex, params.DataBase64, params.ChunkSHA256)
		})
		return result, rpcErr, true
	case "file.upload.status":
		params, err := decodeFileUploadCommitParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32063, func() (any, error) {
			uploader, ok := service.(interface {
				GetAttachmentUploadStatus(uploadID string) (identityusecase.AttachmentUploadStatus, error)
			})
			if !ok {
				return nil, errors.New("chunked file upload is not supported")
			}
			return uploader.GetAttachmentUploadStatus(params.UploadID)
		})
		return result, rpcErr, true
	case "file.upload.commit":
		params, err := decodeFileUploadCommitParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32064, func() (any, error) {
			uploader, ok := service.(interface {
				CommitAttachmentUpload(uploadID string) (models.AttachmentMeta, error)
			})
			if !ok {
				return nil, errors.New("chunked file upload is not supported")
			}
			return uploader.CommitAttachmentUpload(params.UploadID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func callWithFilePutParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(name, mimeType, dataBase64 string) (any, error),
) (any, *rpckit.Error) {
	name, mimeType, dataBase64, err := decodeFilePutParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(name, mimeType, dataBase64)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func decodeFilePutParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 3 {
		if strings.TrimSpace(arr[0]) == "" || strings.TrimSpace(arr[2]) == "" {
			return "", "", "", errors.New("invalid params")
		}
		return arr[0], arr[1], arr[2], nil
	}
	var payload struct {
		Name       string `json:"name"`
		MimeType   string `json:"mime_type"`
		DataBase64 string `json:"data_base64"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", errors.New("invalid params")
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.DataBase64) == "" {
		return "", "", "", errors.New("invalid params")
	}
	return payload.Name, payload.MimeType, payload.DataBase64, nil
}

type fileUploadInitParams struct {
	Name        string `json:"name"`
	MimeType    string `json:"mime_type"`
	TotalSize   int64  `json:"total_size"`
	TotalChunks int    `json:"total_chunks"`
	ChunkSize   int    `json:"chunk_size"`
	FileSHA256  string `json:"file_sha256"`
}

type fileUploadChunkParams struct {
	UploadID    string `json:"upload_id"`
	ChunkIndex  int    `json:"chunk_index"`
	DataBase64  string `json:"data_base64"`
	ChunkSHA256 string `json:"chunk_sha256"`
}

type fileUploadCommitParams struct {
	UploadID string `json:"upload_id"`
}

func decodeFileUploadInitParams(raw json.RawMessage) (fileUploadInitParams, error) {
	var arr []fileUploadInitParams
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		p := arr[0]
		if strings.TrimSpace(p.Name) == "" || p.TotalSize <= 0 || p.TotalChunks <= 0 || p.ChunkSize <= 0 {
			return fileUploadInitParams{}, errors.New("invalid params")
		}
		return p, nil
	}
	var p fileUploadInitParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fileUploadInitParams{}, errors.New("invalid params")
	}
	if strings.TrimSpace(p.Name) == "" || p.TotalSize <= 0 || p.TotalChunks <= 0 || p.ChunkSize <= 0 {
		return fileUploadInitParams{}, errors.New("invalid params")
	}
	return p, nil
}

func decodeFileUploadChunkParams(raw json.RawMessage) (fileUploadChunkParams, error) {
	var arr []fileUploadChunkParams
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		p := arr[0]
		if strings.TrimSpace(p.UploadID) == "" || p.ChunkIndex < 0 || strings.TrimSpace(p.DataBase64) == "" {
			return fileUploadChunkParams{}, errors.New("invalid params")
		}
		return p, nil
	}
	var p fileUploadChunkParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fileUploadChunkParams{}, errors.New("invalid params")
	}
	if strings.TrimSpace(p.UploadID) == "" || p.ChunkIndex < 0 || strings.TrimSpace(p.DataBase64) == "" {
		return fileUploadChunkParams{}, errors.New("invalid params")
	}
	return p, nil
}

func decodeFileUploadCommitParams(raw json.RawMessage) (fileUploadCommitParams, error) {
	var arr []fileUploadCommitParams
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		p := arr[0]
		if strings.TrimSpace(p.UploadID) == "" {
			return fileUploadCommitParams{}, errors.New("invalid params")
		}
		return p, nil
	}
	var p fileUploadCommitParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fileUploadCommitParams{}, errors.New("invalid params")
	}
	if strings.TrimSpace(p.UploadID) == "" {
		return fileUploadCommitParams{}, errors.New("invalid params")
	}
	return p, nil
}
