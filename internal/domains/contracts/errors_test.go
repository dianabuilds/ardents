package contracts

import (
	"errors"
	"testing"
)

func TestWrapCategorizedError_NewErrorUsesProvidedCategory(t *testing.T) {
	wrapped := WrapCategorizedError(ErrorCategoryCrypto, errors.New("boom"))
	var classified *CategorizedError
	if !errors.As(wrapped, &classified) {
		t.Fatalf("expected categorized error, got %T", wrapped)
	}
	if classified.Category != ErrorCategoryCrypto {
		t.Fatalf("expected category=%q, got %q", ErrorCategoryCrypto, classified.Category)
	}
}

func TestWrapCategorizedError_NormalizesUnknownCategoryToAPI(t *testing.T) {
	wrapped := WrapCategorizedError("unknown", errors.New("boom"))
	if got := ErrorCategory(wrapped); got != ErrorCategoryAPI {
		t.Fatalf("expected category=%q, got %q", ErrorCategoryAPI, got)
	}
}

func TestErrorCategory_DefaultsToAPIForRegularErrors(t *testing.T) {
	if got := ErrorCategory(errors.New("plain")); got != ErrorCategoryAPI {
		t.Fatalf("expected default category=%q, got %q", ErrorCategoryAPI, got)
	}
}
