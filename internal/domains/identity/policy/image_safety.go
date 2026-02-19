package policy

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
)

const (
	maxImageWidth  = 8192
	maxImageHeight = 8192
	maxImagePixels = 30_000_000
	jpegQuality    = 85
)

var errInvalidImagePayload = errors.New("invalid image payload")

func normalizeAttachmentPayload(name, mimeType string, data []byte, maxBytes int) (string, string, []byte, error) {
	name = strings.TrimSpace(name)
	mimeType = strings.TrimSpace(mimeType)
	if name == "" || len(data) == 0 {
		return "", "", nil, errors.New("attachment name and data are required")
	}
	if len(data) > maxBytes {
		return "", "", nil, errors.New("attachment exceeds maximum size")
	}

	if !isClaimedImageMime(mimeType) {
		return name, mimeType, data, nil
	}

	detectedMime := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
	if !isSupportedImageMime(detectedMime) {
		return "", "", nil, errInvalidImagePayload
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", "", nil, errInvalidImagePayload
	}
	if !isSafeImageBounds(cfg.Width, cfg.Height) {
		return "", "", nil, errors.New("image dimensions exceed safety limits")
	}

	canonicalMime, ok := imageFormatToMime(format)
	if !ok {
		return "", "", nil, errInvalidImagePayload
	}

	normalized := data
	switch format {
	case "jpeg":
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", "", nil, errInvalidImagePayload
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return "", "", nil, errInvalidImagePayload
		}
		normalized = buf.Bytes()
	case "png":
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", "", nil, errInvalidImagePayload
		}
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return "", "", nil, errInvalidImagePayload
		}
		normalized = buf.Bytes()
	case "gif":
		// Keep original bytes to preserve animated GIFs.
	}

	if len(normalized) == 0 || len(normalized) > maxBytes {
		return "", "", nil, errors.New("attachment exceeds maximum size")
	}
	return name, canonicalMime, normalized, nil
}

func NormalizeDirectAttachmentPayload(name, mimeType string, data []byte) (string, string, []byte, error) {
	return normalizeAttachmentPayload(name, mimeType, data, maxAttachmentBytes)
}

func NormalizeChunkedAttachmentPayload(name, mimeType string, data []byte) (string, string, []byte, error) {
	return normalizeAttachmentPayload(name, mimeType, data, maxChunkedAttachmentBytes)
}

func isClaimedImageMime(mimeType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/")
}

func isSupportedImageMime(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif":
		return true
	default:
		return false
	}
}

func imageFormatToMime(format string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg":
		return "image/jpeg", true
	case "png":
		return "image/png", true
	case "gif":
		return "image/gif", true
	default:
		return "", false
	}
}

func isSafeImageBounds(width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	if width > maxImageWidth || height > maxImageHeight {
		return false
	}
	return int64(width)*int64(height) <= maxImagePixels
}
