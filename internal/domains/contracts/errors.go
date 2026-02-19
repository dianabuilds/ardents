package contracts

import (
	"errors"
	"strings"
)

var ErrAttachmentTemporarilyUnavailable = errors.New("attachment is temporarily unavailable")
var ErrAttachmentAccessDenied = errors.New("attachment access denied")

const (
	ErrorCategoryAPI     = "api"
	ErrorCategoryCrypto  = "crypto"
	ErrorCategoryStorage = "storage"
	ErrorCategoryNetwork = "network"
)

func normalizeErrorCategory(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case ErrorCategoryCrypto:
		return ErrorCategoryCrypto
	case ErrorCategoryStorage:
		return ErrorCategoryStorage
	case ErrorCategoryNetwork:
		return ErrorCategoryNetwork
	default:
		return ErrorCategoryAPI
	}
}

func WrapCategorizedError(category string, err error) error {
	if err == nil {
		return nil
	}
	var existing *CategorizedError
	if errors.As(err, &existing) {
		return &CategorizedError{
			Category: normalizeErrorCategory(existing.Category),
			Err:      existing.Err,
		}
	}
	return &CategorizedError{
		Category: normalizeErrorCategory(category),
		Err:      err,
	}
}

func ErrorCategory(err error) string {
	var classified *CategorizedError
	if errors.As(err, &classified) {
		return normalizeErrorCategory(classified.Category)
	}
	return ErrorCategoryAPI
}
