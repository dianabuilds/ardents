package rpc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"aim-chat/go-backend/pkg/models"
)

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
	return zero, errInvalidParams
}

func intPtrValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

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

func decodeThreeStringParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 3 && arr[0] != "" && arr[1] != "" && arr[2] != "" {
		return arr[0], arr[1], arr[2], nil
	}
	return "", "", "", errInvalidParams
}

func decodeFourStringParams(raw json.RawMessage) (string, string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 4 || arr[0] == "" || arr[1] == "" {
		return "", "", "", "", errInvalidParams
	}
	return arr[0], arr[1], arr[2], arr[3], nil
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
			return "", "", 0, 0, 0, 0, 0, 0, 0, errInvalidParams
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
	return "", "", 0, 0, 0, 0, 0, 0, 0, errInvalidParams
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
			return "", "", "", "", 0, 0, 0, 0, 0, 0, 0, false, false, errInvalidParams
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
	return "", "", "", "", 0, 0, 0, 0, 0, 0, 0, false, false, errInvalidParams
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
			return "", "", false, errInvalidParams
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
	return "", "", false, errInvalidParams
}

func decodeBlobProvidersParams(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		blobID := strings.TrimSpace(arr[0])
		if blobID == "" {
			return "", errInvalidParams
		}
		return blobID, nil
	}
	var payload struct {
		BlobID string `json:"blob_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", errInvalidParams
	}
	blobID := strings.TrimSpace(payload.BlobID)
	if blobID == "" {
		return "", errInvalidParams
	}
	return blobID, nil
}

func decodeBlobReplicationModeParams(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		mode := strings.TrimSpace(arr[0])
		if mode == "" {
			return "", errInvalidParams
		}
		return mode, nil
	}
	var payload struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", errInvalidParams
	}
	mode := strings.TrimSpace(payload.Mode)
	if mode == "" {
		return "", errInvalidParams
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
			return false, false, 0, errInvalidParams
		}
		if *p.RolloutPercent < 0 || *p.RolloutPercent > 100 {
			return false, false, 0, errInvalidParams
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
	return false, false, 0, errInvalidParams
}

func decodeBlobACLPolicyParams(raw json.RawMessage) (string, []string, error) {
	type payload struct {
		Mode      string   `json:"mode"`
		Allowlist []string `json:"allowlist"`
	}
	parse := func(p payload) (string, []string, error) {
		mode := strings.TrimSpace(p.Mode)
		if mode == "" {
			return "", nil, errInvalidParams
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
	return "", nil, errInvalidParams
}

func decodeBlobNodePresetParams(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		preset := strings.TrimSpace(arr[0])
		if preset == "" {
			return "", errInvalidParams
		}
		return preset, nil
	}
	var payload struct {
		Preset string `json:"preset"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", errInvalidParams
	}
	preset := strings.TrimSpace(payload.Preset)
	if preset == "" {
		return "", errInvalidParams
	}
	return preset, nil
}

func decodeNodeBindingLinkCreateParams(raw json.RawMessage) (int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	type payload struct {
		TTLSeconds *int `json:"ttl_seconds"`
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		if arr[0].TTLSeconds == nil {
			return 0, nil
		}
		if *arr[0].TTLSeconds < 1 {
			return 0, errInvalidParams
		}
		return *arr[0].TTLSeconds, nil
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		if direct.TTLSeconds == nil {
			return 0, nil
		}
		if *direct.TTLSeconds < 1 {
			return 0, errInvalidParams
		}
		return *direct.TTLSeconds, nil
	}
	return 0, errInvalidParams
}

func decodeNodeBindingCompleteParams(raw json.RawMessage) (string, string, string, string, bool, error) {
	type payload struct {
		LinkCode            string `json:"link_code"`
		NodeID              string `json:"node_id"`
		NodePublicKeyBase64 string `json:"node_public_key_base64"`
		NodeSignatureBase64 string `json:"node_signature_base64"`
		Rebind              bool   `json:"rebind"`
	}
	parse := func(p payload) (string, string, string, string, bool, error) {
		linkCode := strings.TrimSpace(p.LinkCode)
		nodeID := strings.TrimSpace(p.NodeID)
		pub := strings.TrimSpace(p.NodePublicKeyBase64)
		sig := strings.TrimSpace(p.NodeSignatureBase64)
		if linkCode == "" || nodeID == "" || pub == "" || sig == "" {
			return "", "", "", "", false, errInvalidParams
		}
		return linkCode, nodeID, pub, sig, p.Rebind, nil
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return parse(arr[0])
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parse(direct)
	}
	return "", "", "", "", false, errInvalidParams
}

func decodeNodeBindingUnbindParams(raw json.RawMessage) (string, bool, error) {
	type payload struct {
		NodeID  string `json:"node_id"`
		Confirm bool   `json:"confirm"`
	}
	parse := func(p payload) (string, bool, error) {
		return strings.TrimSpace(p.NodeID), p.Confirm, nil
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return parse(arr[0])
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parse(direct)
	}
	return "", false, errInvalidParams
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
