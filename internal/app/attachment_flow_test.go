package app

import (
	"encoding/base64"
	"testing"
)

func TestDecodeAttachmentInput(t *testing.T) {
	raw := []byte("hello")
	enc := base64.StdEncoding.EncodeToString(raw)
	name, mime, data, err := DecodeAttachmentInput(" file.txt ", " text/plain ", enc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "file.txt" || mime != "text/plain" || string(data) != "hello" {
		t.Fatalf("unexpected decoded values: %q %q %q", name, mime, string(data))
	}
}

func TestDecodeAttachmentInput_Invalid(t *testing.T) {
	if _, _, _, err := DecodeAttachmentInput("", "text/plain", "abc"); err == nil {
		t.Fatal("expected validation error")
	}
	if _, _, _, err := DecodeAttachmentInput("a", "text/plain", "%%%"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestValidateAttachmentID(t *testing.T) {
	id, err := ValidateAttachmentID(" att-1 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "att-1" {
		t.Fatalf("unexpected id: %q", id)
	}
	if _, err := ValidateAttachmentID("  "); err == nil {
		t.Fatal("expected empty id error")
	}
}
