package policy

import (
	"encoding/base64"
	"errors"
	"strings"
)

const (
	maxAttachmentBytes        = 5 * 1024 * 1024
	maxChunkedAttachmentMB    = 64
	maxChunkedAttachmentBytes = maxChunkedAttachmentMB * 1024 * 1024
	minAttachmentChunkSize    = 16 * 1024
	maxAttachmentChunkSize    = 1024 * 1024
)

func DecodeAttachmentInput(name, mimeType, dataBase64 string) (string, string, []byte, error) {
	name = strings.TrimSpace(name)
	mimeType = strings.TrimSpace(mimeType)
	dataBase64 = strings.TrimSpace(dataBase64)
	if name == "" || dataBase64 == "" {
		return "", "", nil, errors.New("attachment name and data are required")
	}
	if base64.StdEncoding.DecodedLen(len(dataBase64)) > maxAttachmentBytes {
		return "", "", nil, errors.New("attachment exceeds maximum size")
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return "", "", nil, errors.New("invalid attachment encoding")
	}
	if len(data) > maxAttachmentBytes {
		return "", "", nil, errors.New("attachment exceeds maximum size")
	}
	return name, mimeType, data, nil
}

func ValidateAttachmentID(attachmentID string) (string, error) {
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		return "", errors.New("attachment id is required")
	}
	return attachmentID, nil
}

func ValidateAttachmentUploadInit(name, mimeType string, totalSize int64, totalChunks, chunkSize int) (string, string, int64, int, int, error) {
	name = strings.TrimSpace(name)
	mimeType = strings.TrimSpace(mimeType)
	if name == "" {
		return "", "", 0, 0, 0, errors.New("attachment name is required")
	}
	if totalSize <= 0 || totalSize > maxChunkedAttachmentBytes {
		return "", "", 0, 0, 0, errors.New("attachment exceeds maximum size")
	}
	if totalChunks <= 0 || totalChunks > 10_000 {
		return "", "", 0, 0, 0, errors.New("invalid chunk count")
	}
	if chunkSize < minAttachmentChunkSize || chunkSize > maxAttachmentChunkSize {
		return "", "", 0, 0, 0, errors.New("invalid chunk size")
	}
	if int64(totalChunks-1)*int64(chunkSize) >= totalSize {
		return "", "", 0, 0, 0, errors.New("invalid chunk manifest")
	}
	return name, mimeType, totalSize, totalChunks, chunkSize, nil
}

func ValidateAttachmentChunkInput(uploadID string, index int, data []byte, expectedChunkSize int, totalSize int64, totalChunks int) (string, error) {
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return "", errors.New("upload id is required")
	}
	if index < 0 || index >= totalChunks {
		return "", errors.New("invalid chunk index")
	}
	if len(data) == 0 {
		return "", errors.New("chunk data is required")
	}
	if index < totalChunks-1 && len(data) != expectedChunkSize {
		return "", errors.New("invalid chunk size")
	}
	if index == totalChunks-1 {
		remaining := int(totalSize - int64(expectedChunkSize*(totalChunks-1)))
		if remaining <= 0 || len(data) != remaining {
			return "", errors.New("invalid final chunk size")
		}
	}
	return uploadID, nil
}

func ValidateLoginInput(accountID, password, currentIdentityID string) error {
	accountID = strings.TrimSpace(accountID)
	password = strings.TrimSpace(password)
	if accountID == "" || password == "" {
		return errors.New("account id and password are required")
	}
	if strings.TrimSpace(currentIdentityID) != accountID {
		return errors.New("account id mismatch")
	}
	return nil
}
