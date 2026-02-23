package networkmanifest

import (
	"aim-chat/go-backend/internal/bootstrap/manifesttrust"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

type RejectCode string

const (
	RejectSchemaInvalid    RejectCode = "MANIFEST_SCHEMA_INVALID"
	RejectExpired          RejectCode = "MANIFEST_EXPIRED"
	RejectReplay           RejectCode = "MANIFEST_REPLAY_DETECTED"
	RejectKeyUnknown       RejectCode = "MANIFEST_KEY_UNKNOWN"
	RejectSignatureInvalid RejectCode = "MANIFEST_SIGNATURE_INVALID"
	RejectPolicyInvalid    RejectCode = "MANIFEST_POLICY_INVALID"
)

var keyIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)

type ReconnectPolicy struct {
	BaseIntervalMS int     `json:"base_interval_ms"`
	MaxIntervalMS  int     `json:"max_interval_ms"`
	JitterRatio    float64 `json:"jitter_ratio"`
}

type Manifest struct {
	Version         int             `json:"version"`
	GeneratedAt     time.Time       `json:"generated_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	BootstrapNodes  []string        `json:"bootstrap_nodes"`
	MinPeers        int             `json:"min_peers"`
	ReconnectPolicy ReconnectPolicy `json:"reconnect_policy"`
	KeyID           string          `json:"key_id"`
	Signature       string          `json:"signature"`
}

type VerifyError struct {
	Code RejectCode
	Err  error
}

func (e *VerifyError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *VerifyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func RejectCodeOf(err error) (RejectCode, bool) {
	var verr *VerifyError
	if errors.As(err, &verr) {
		return verr.Code, true
	}
	return "", false
}

type VerifyRequest struct {
	Raw                []byte
	TrustBundle        manifesttrust.Bundle
	Now                time.Time
	LastAppliedVersion int
}

func ParseStrict(raw []byte) (Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, &VerifyError{Code: RejectSchemaInvalid, Err: err}
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return Manifest{}, &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("unexpected trailing json tokens")}
	} else if !errors.Is(err, io.EOF) {
		return Manifest{}, &VerifyError{Code: RejectSchemaInvalid, Err: err}
	}
	if err := validateSchema(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func Verify(req VerifyRequest) (Manifest, error) {
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}
	manifest, err := ParseStrict(req.Raw)
	if err != nil {
		return Manifest{}, err
	}
	if !manifest.ExpiresAt.After(req.Now) {
		return Manifest{}, &VerifyError{Code: RejectExpired, Err: errors.New("manifest expired")}
	}
	if req.LastAppliedVersion > 0 && manifest.Version < req.LastAppliedVersion {
		return Manifest{}, &VerifyError{Code: RejectReplay, Err: errors.New("manifest version is older than last applied")}
	}

	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil {
		return Manifest{}, &VerifyError{Code: RejectSignatureInvalid, Err: errors.New("signature is not valid base64")}
	}
	payload, err := CanonicalPayload(manifest)
	if err != nil {
		return Manifest{}, &VerifyError{Code: RejectSchemaInvalid, Err: err}
	}
	if err := manifesttrust.VerifyManifestSignature(req.TrustBundle, manifest.KeyID, payload, signature, req.Now); err != nil {
		switch {
		case errors.Is(err, manifesttrust.ErrManifestKeyUnknown), errors.Is(err, manifesttrust.ErrManifestKeyInactive):
			return Manifest{}, &VerifyError{Code: RejectKeyUnknown, Err: err}
		case errors.Is(err, manifesttrust.ErrManifestSignatureInvalid):
			return Manifest{}, &VerifyError{Code: RejectSignatureInvalid, Err: err}
		default:
			return Manifest{}, &VerifyError{Code: RejectSchemaInvalid, Err: err}
		}
	}
	return manifest, nil
}

func CanonicalPayload(m Manifest) ([]byte, error) {
	v := map[string]any{
		"version":         m.Version,
		"generated_at":    m.GeneratedAt.UTC().Format(time.RFC3339Nano),
		"expires_at":      m.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"bootstrap_nodes": m.BootstrapNodes,
		"min_peers":       m.MinPeers,
		"reconnect_policy": map[string]any{
			"base_interval_ms": m.ReconnectPolicy.BaseIntervalMS,
			"max_interval_ms":  m.ReconnectPolicy.MaxIntervalMS,
			"jitter_ratio":     m.ReconnectPolicy.JitterRatio,
		},
		"key_id": m.KeyID,
	}
	return json.Marshal(v)
}

func canonicalPayload(m Manifest) ([]byte, error) {
	return CanonicalPayload(m)
}

func validateSchema(m Manifest) error {
	if m.Version < 1 {
		return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("version must be >= 1")}
	}
	if m.GeneratedAt.IsZero() {
		return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("generated_at is required")}
	}
	if m.ExpiresAt.IsZero() {
		return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("expires_at is required")}
	}
	if !m.ExpiresAt.After(m.GeneratedAt) {
		return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("expires_at must be greater than generated_at")}
	}
	if len(m.BootstrapNodes) < 1 || len(m.BootstrapNodes) > 64 {
		return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("bootstrap_nodes size must be within [1..64]")}
	}
	for _, node := range m.BootstrapNodes {
		trimmed := strings.TrimSpace(node)
		if trimmed == "" || len(trimmed) > 512 {
			return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("bootstrap node entry is invalid")}
		}
	}
	if m.MinPeers < 1 || m.MinPeers > 128 {
		return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("min_peers must be within [1..128]")}
	}
	if m.ReconnectPolicy.BaseIntervalMS < 500 || m.ReconnectPolicy.BaseIntervalMS > 120000 {
		return &VerifyError{Code: RejectPolicyInvalid, Err: errors.New("base_interval_ms must be within [500..120000]")}
	}
	if m.ReconnectPolicy.MaxIntervalMS < 500 || m.ReconnectPolicy.MaxIntervalMS > 300000 {
		return &VerifyError{Code: RejectPolicyInvalid, Err: errors.New("max_interval_ms must be within [500..300000]")}
	}
	if m.ReconnectPolicy.MaxIntervalMS < m.ReconnectPolicy.BaseIntervalMS {
		return &VerifyError{Code: RejectPolicyInvalid, Err: errors.New("max_interval_ms must be >= base_interval_ms")}
	}
	if m.ReconnectPolicy.JitterRatio < 0 || m.ReconnectPolicy.JitterRatio > 1 {
		return &VerifyError{Code: RejectPolicyInvalid, Err: errors.New("jitter_ratio must be within [0..1]")}
	}
	if m.KeyID == "" || len(m.KeyID) > 128 || !keyIDPattern.MatchString(m.KeyID) {
		return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("key_id is invalid")}
	}
	if l := len(m.Signature); l < 16 || l > 4096 {
		return &VerifyError{Code: RejectSchemaInvalid, Err: errors.New("signature length is invalid")}
	}
	return nil
}
