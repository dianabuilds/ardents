package rpc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"aim-chat/go-backend/pkg/models"
)

func decodeCardParam(raw json.RawMessage) (models.ContactCard, error) {
	// Preferred shape: [ { ...card } ]
	var arr []models.ContactCard
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return arr[0], nil
	}

	// Alternative shape: { "card": { ... } }
	var wrapper struct {
		Card models.ContactCard `json:"card"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Card.IdentityID != "" {
		return wrapper.Card, nil
	}

	return models.ContactCard{}, errInvalidParams
}

func decodeAddContactParams(raw json.RawMessage) (models.ContactCard, string, string, error) {
	if card, err := decodeCardParam(raw); err == nil {
		return card, "", "", nil
	}
	contactID, displayName, err := decodeContactByIDParams(raw)
	if err != nil {
		return models.ContactCard{}, "", "", errInvalidParams
	}
	return models.ContactCard{}, contactID, displayName, nil
}

func decodeSingleStringParam(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 && arr[0] != "" {
		return arr[0], nil
	}
	return "", errInvalidParams
}

func decodeTwoStringParams(raw json.RawMessage) (string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 2 && arr[0] != "" && arr[1] != "" {
		return arr[0], arr[1], nil
	}
	return "", "", errInvalidParams
}

func decodeSessionInitParams(raw json.RawMessage) (string, []byte, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 2 || arr[0] == "" || arr[1] == "" {
		return "", nil, errInvalidParams
	}
	peerKey, err := base64.StdEncoding.DecodeString(arr[1])
	if err != nil {
		return "", nil, errInvalidParams
	}
	return arr[0], peerKey, nil
}

func decodeMessageListParams(raw json.RawMessage) (string, int, int, error) {
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 3 {
		return "", 0, 0, errInvalidParams
	}
	contactID, ok := arr[0].(string)
	if !ok || contactID == "" {
		return "", 0, 0, errInvalidParams
	}
	limit, err := decodeStrictNonNegativeInt(arr[1])
	if err != nil {
		return "", 0, 0, errInvalidParams
	}
	offset, err := decodeStrictNonNegativeInt(arr[2])
	if err != nil {
		return "", 0, 0, errInvalidParams
	}
	if limit > maxMessageListLimit || offset > maxMessageListOffset {
		return "", 0, 0, errInvalidParams
	}
	return contactID, limit, offset, nil
}

func decodeThreadSendParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 3 {
		return "", "", "", errInvalidParams
	}
	targetID := strings.TrimSpace(arr[0])
	content := strings.TrimSpace(arr[1])
	threadID := strings.TrimSpace(arr[2])
	if targetID == "" || content == "" || threadID == "" {
		return "", "", "", errInvalidParams
	}
	return targetID, content, threadID, nil
}

func decodeThreadListParams(raw json.RawMessage) (string, string, int, int, error) {
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 4 {
		return "", "", 0, 0, errInvalidParams
	}
	targetID, ok := arr[0].(string)
	if !ok || strings.TrimSpace(targetID) == "" {
		return "", "", 0, 0, errInvalidParams
	}
	threadID, ok := arr[1].(string)
	if !ok || strings.TrimSpace(threadID) == "" {
		return "", "", 0, 0, errInvalidParams
	}
	limit, err := decodeStrictNonNegativeInt(arr[2])
	if err != nil {
		return "", "", 0, 0, errInvalidParams
	}
	offset, err := decodeStrictNonNegativeInt(arr[3])
	if err != nil {
		return "", "", 0, 0, errInvalidParams
	}
	if limit > maxMessageListLimit || offset > maxMessageListOffset {
		return "", "", 0, 0, errInvalidParams
	}
	return strings.TrimSpace(targetID), strings.TrimSpace(threadID), limit, offset, nil
}

func decodeStrictNonNegativeInt(raw any) (int, error) {
	v, ok := raw.(float64)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errInvalidParams
	}
	if v < 0 || math.Trunc(v) != v {
		return 0, errInvalidParams
	}
	maxInt := float64(^uint(0) >> 1)
	if v > maxInt {
		return 0, errInvalidParams
	}
	return int(v), nil
}

func decodeContactByIDParams(raw json.RawMessage) (string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return "", "", errInvalidParams
	}
	if len(arr) == 1 && arr[0] != "" {
		return arr[0], "", nil
	}
	if len(arr) == 2 && arr[0] != "" {
		return arr[0], arr[1], nil
	}
	return "", "", errInvalidParams
}

func decodeMessageEditParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 3 {
		return "", "", "", errInvalidParams
	}
	if strings.TrimSpace(arr[0]) == "" || strings.TrimSpace(arr[1]) == "" || strings.TrimSpace(arr[2]) == "" {
		return "", "", "", errInvalidParams
	}
	return arr[0], arr[1], arr[2], nil
}

func decodeFilePutParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 3 {
		if strings.TrimSpace(arr[0]) == "" || strings.TrimSpace(arr[2]) == "" {
			return "", "", "", errInvalidParams
		}
		return arr[0], arr[1], arr[2], nil
	}

	var payload struct {
		Name       string `json:"name"`
		MimeType   string `json:"mime_type"`
		DataBase64 string `json:"data_base64"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", errInvalidParams
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.DataBase64) == "" {
		return "", "", "", errInvalidParams
	}
	return payload.Name, payload.MimeType, payload.DataBase64, nil
}

func decodeChannelCreateParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		switch len(arr) {
		case 1:
			name := strings.TrimSpace(arr[0])
			if name == "" {
				return "", "", "", errInvalidParams
			}
			return name, "public", "", nil
		case 2:
			name := strings.TrimSpace(arr[0])
			visibility := strings.TrimSpace(arr[1])
			if name == "" || visibility == "" {
				return "", "", "", errInvalidParams
			}
			return name, visibility, "", nil
		case 3:
			name := strings.TrimSpace(arr[0])
			visibility := strings.TrimSpace(arr[1])
			description := strings.TrimSpace(arr[2])
			if name == "" || visibility == "" {
				return "", "", "", errInvalidParams
			}
			return name, visibility, description, nil
		}
	}

	var payload struct {
		Name        string `json:"name"`
		Visibility  string `json:"visibility"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", errInvalidParams
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return "", "", "", errInvalidParams
	}
	visibility := strings.TrimSpace(payload.Visibility)
	if visibility == "" {
		visibility = "public"
	}
	return name, visibility, strings.TrimSpace(payload.Description), nil
}

func decodeStoragePolicyParams(raw json.RawMessage) (string, string, int, int, error) {
	type payload struct {
		StorageProtectionMode string `json:"storage_protection_mode"`
		ContentRetentionMode  string `json:"content_retention_mode"`
		MessageTTLSeconds     *int   `json:"message_ttl_seconds"`
		FileTTLSeconds        *int   `json:"file_ttl_seconds"`
	}

	decodePayload := func(p payload) (string, string, int, int, error) {
		protection := strings.TrimSpace(p.StorageProtectionMode)
		retention := strings.TrimSpace(p.ContentRetentionMode)
		if protection == "" || retention == "" {
			return "", "", 0, 0, errInvalidParams
		}
		messageTTL := 0
		fileTTL := 0
		if p.MessageTTLSeconds != nil {
			messageTTL = *p.MessageTTLSeconds
		}
		if p.FileTTLSeconds != nil {
			fileTTL = *p.FileTTLSeconds
		}
		return protection, retention, messageTTL, fileTTL, nil
	}

	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return decodePayload(arr[0])
	}

	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return decodePayload(direct)
	}
	return "", "", 0, 0, errInvalidParams
}

var errInvalidParams = errors.New("invalid params")

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
			return fileUploadInitParams{}, errInvalidParams
		}
		return p, nil
	}
	var p fileUploadInitParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fileUploadInitParams{}, errInvalidParams
	}
	if strings.TrimSpace(p.Name) == "" || p.TotalSize <= 0 || p.TotalChunks <= 0 || p.ChunkSize <= 0 {
		return fileUploadInitParams{}, errInvalidParams
	}
	return p, nil
}

func decodeFileUploadChunkParams(raw json.RawMessage) (fileUploadChunkParams, error) {
	var arr []fileUploadChunkParams
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		p := arr[0]
		if strings.TrimSpace(p.UploadID) == "" || p.ChunkIndex < 0 || strings.TrimSpace(p.DataBase64) == "" {
			return fileUploadChunkParams{}, errInvalidParams
		}
		return p, nil
	}
	var p fileUploadChunkParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fileUploadChunkParams{}, errInvalidParams
	}
	if strings.TrimSpace(p.UploadID) == "" || p.ChunkIndex < 0 || strings.TrimSpace(p.DataBase64) == "" {
		return fileUploadChunkParams{}, errInvalidParams
	}
	return p, nil
}

func decodeFileUploadCommitParams(raw json.RawMessage) (fileUploadCommitParams, error) {
	var arr []fileUploadCommitParams
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		p := arr[0]
		if strings.TrimSpace(p.UploadID) == "" {
			return fileUploadCommitParams{}, errInvalidParams
		}
		return p, nil
	}
	var p fileUploadCommitParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fileUploadCommitParams{}, errInvalidParams
	}
	if strings.TrimSpace(p.UploadID) == "" {
		return fileUploadCommitParams{}, errInvalidParams
	}
	return p, nil
}
