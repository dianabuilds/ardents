package identity

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
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
	bad := base64.StdEncoding.EncodeToString([]byte("not-an-image"))
	if _, _, _, err := DecodeAttachmentInput("photo.png", "image/png", bad); err == nil {
		t.Fatal("expected invalid image payload error")
	}
}

func TestDecodeAttachmentInput_ImageMimeNormalizedByPayload(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	img.Set(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	name, mime, data, err := DecodeAttachmentInput(
		"photo.jpg",
		"image/png",
		base64.StdEncoding.EncodeToString(buf.Bytes()),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "photo.jpg" {
		t.Fatalf("unexpected name: %q", name)
	}
	if mime != "image/jpeg" {
		t.Fatalf("unexpected normalized mime: %q", mime)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty normalized payload")
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

func TestDecodeAttachmentInput_RejectsUnsafeImageDimensions(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 9000, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	_, _, _, err := DecodeAttachmentInput("wide.png", "image/png", base64.StdEncoding.EncodeToString(buf.Bytes()))
	if err == nil {
		t.Fatal("expected unsafe image dimensions error")
	}
}

func TestDecodeAttachmentInput_StripsPolyglotLikeTailForJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	tail := []byte("<?php echo 'polyglot'; ?>")
	polyglot := append(buf.Bytes(), tail...)

	_, mime, normalized, err := DecodeAttachmentInput("photo.jpg", "image/jpeg", base64.StdEncoding.EncodeToString(polyglot))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("unexpected normalized mime: %q", mime)
	}
	if bytes.Contains(normalized, tail) {
		t.Fatal("normalized image must not preserve appended payload tail")
	}
}
