package policy

import (
	"encoding/base64"
	"errors"
	"strings"
)

const maxAttachmentBytes = 5 * 1024 * 1024

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
