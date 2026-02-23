package manifesttrust

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrTrustBundleInvalid          = errors.New("trust bundle invalid")
	ErrTrustUpdateChainInvalid     = errors.New("trust update chain invalid")
	ErrTrustUpdateSignatureInvalid = errors.New("trust update signature invalid")
	ErrManifestKeyUnknown          = errors.New("manifest key unknown")
	ErrManifestKeyInactive         = errors.New("manifest key inactive")
	ErrManifestSignatureInvalid    = errors.New("manifest signature invalid")
)

type RootKey struct {
	KeyID           string `json:"key_id"`
	Algorithm       string `json:"algorithm"`
	PublicKeyBase64 string `json:"public_key_base64"`
}

type ManifestKey struct {
	KeyID           string    `json:"key_id"`
	Algorithm       string    `json:"algorithm"`
	PublicKeyBase64 string    `json:"public_key_base64"`
	NotBefore       time.Time `json:"not_before"`
	NotAfter        time.Time `json:"not_after"`
}

func (k ManifestKey) IsActive(at time.Time) bool {
	if k.NotBefore.IsZero() || k.NotAfter.IsZero() {
		return false
	}
	return !at.Before(k.NotBefore) && at.Before(k.NotAfter)
}

type Bundle struct {
	Version      int           `json:"version"`
	BundleID     string        `json:"bundle_id"`
	GeneratedAt  time.Time     `json:"generated_at"`
	RootKeys     []RootKey     `json:"root_keys"`
	ManifestKeys []ManifestKey `json:"manifest_keys"`
}

func ParseBundle(data []byte) (Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return Bundle{}, fmt.Errorf("%w: %v", ErrTrustBundleInvalid, err)
	}
	if err := b.Validate(); err != nil {
		return Bundle{}, err
	}
	return b, nil
}

func (b Bundle) Validate() error {
	if b.Version <= 0 {
		return fmt.Errorf("%w: version must be > 0", ErrTrustBundleInvalid)
	}
	if b.BundleID == "" {
		return fmt.Errorf("%w: bundle_id is required", ErrTrustBundleInvalid)
	}
	if b.GeneratedAt.IsZero() {
		return fmt.Errorf("%w: generated_at is required", ErrTrustBundleInvalid)
	}
	if len(b.RootKeys) == 0 {
		return fmt.Errorf("%w: root_keys are required", ErrTrustBundleInvalid)
	}
	if len(b.ManifestKeys) == 0 {
		return fmt.Errorf("%w: manifest_keys are required", ErrTrustBundleInvalid)
	}
	for _, key := range b.RootKeys {
		if err := validatePublicKeyFields(key.KeyID, key.Algorithm, key.PublicKeyBase64); err != nil {
			return err
		}
	}
	for _, key := range b.ManifestKeys {
		if err := validatePublicKeyFields(key.KeyID, key.Algorithm, key.PublicKeyBase64); err != nil {
			return err
		}
		if key.NotBefore.IsZero() || key.NotAfter.IsZero() || !key.NotAfter.After(key.NotBefore) {
			return fmt.Errorf("%w: invalid manifest key validity window", ErrTrustBundleInvalid)
		}
	}
	return nil
}

func validatePublicKeyFields(keyID, algorithm, publicKeyBase64 string) error {
	if keyID == "" {
		return fmt.Errorf("%w: key_id is required", ErrTrustBundleInvalid)
	}
	if algorithm != "ed25519" {
		return fmt.Errorf("%w: unsupported algorithm %q", ErrTrustBundleInvalid, algorithm)
	}
	raw, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return fmt.Errorf("%w: invalid public key base64 for %q", ErrTrustBundleInvalid, keyID)
	}
	if len(raw) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid public key size for %q", ErrTrustBundleInvalid, keyID)
	}
	return nil
}

func (b Bundle) hasRootKeyByID(keyID string) bool {
	for _, k := range b.RootKeys {
		if k.KeyID == keyID {
			return true
		}
	}
	return false
}

func (b Bundle) findManifestKey(keyID string) (ManifestKey, bool) {
	for _, k := range b.ManifestKeys {
		if k.KeyID == keyID {
			return k, true
		}
	}
	return ManifestKey{}, false
}

func (b Bundle) ActiveManifestKeys(at time.Time) []ManifestKey {
	out := make([]ManifestKey, 0, len(b.ManifestKeys))
	for _, k := range b.ManifestKeys {
		if k.IsActive(at) {
			out = append(out, k)
		}
	}
	return out
}

func VerifyManifestSignature(b Bundle, keyID string, payload, signature []byte, at time.Time) error {
	key, ok := b.findManifestKey(keyID)
	if !ok {
		return ErrManifestKeyUnknown
	}
	if !key.IsActive(at) {
		return ErrManifestKeyInactive
	}
	pub, err := decodeEd25519PublicKey(key.PublicKeyBase64)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTrustBundleInvalid, err)
	}
	if !ed25519.Verify(pub, payload, signature) {
		return ErrManifestSignatureInvalid
	}
	return nil
}

func decodeEd25519PublicKey(value string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid key size")
	}
	return raw, nil
}

type BundleUpdateEnvelope struct {
	Bundle          Bundle `json:"bundle"`
	SignedByKeyID   string `json:"signed_by_key_id"`
	SignatureBase64 string `json:"signature_base64"`
}

func (e BundleUpdateEnvelope) Validate() error {
	if err := e.Bundle.Validate(); err != nil {
		return err
	}
	if e.SignedByKeyID == "" {
		return fmt.Errorf("%w: signed_by_key_id is required", ErrTrustBundleInvalid)
	}
	if e.SignatureBase64 == "" {
		return fmt.Errorf("%w: signature_base64 is required", ErrTrustBundleInvalid)
	}
	return nil
}

func VerifyAndApplyUpdate(current Bundle, update BundleUpdateEnvelope, at time.Time) (Bundle, error) {
	if err := current.Validate(); err != nil {
		return Bundle{}, err
	}
	if err := update.Validate(); err != nil {
		return Bundle{}, err
	}
	if !current.hasRootKeyByID(update.SignedByKeyID) {
		return Bundle{}, ErrTrustUpdateChainInvalid
	}

	signerRoot, ok := findRootKey(current.RootKeys, update.SignedByKeyID)
	if !ok {
		return Bundle{}, ErrTrustUpdateChainInvalid
	}
	if err := verifyBundleUpdateSignature(signerRoot, update); err != nil {
		return Bundle{}, err
	}
	if err := verifyRootContinuity(current, update.Bundle); err != nil {
		return Bundle{}, err
	}
	if err := verifyBundleMonotonic(current, update.Bundle); err != nil {
		return Bundle{}, err
	}
	if err := verifyActiveManifestKeys(update.Bundle, at); err != nil {
		return Bundle{}, err
	}
	if err := verifyRotationOverlap(current, update.Bundle, at); err != nil {
		return Bundle{}, err
	}
	return update.Bundle, nil
}

func findRootKey(keys []RootKey, keyID string) (RootKey, bool) {
	for _, k := range keys {
		if k.KeyID == keyID {
			return k, true
		}
	}
	return RootKey{}, false
}

func verifyBundleUpdateSignature(root RootKey, update BundleUpdateEnvelope) error {
	pub, err := decodeEd25519PublicKey(root.PublicKeyBase64)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTrustBundleInvalid, err)
	}
	msg, err := canonicalBundlePayload(update.Bundle)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTrustBundleInvalid, err)
	}
	sig, err := base64.StdEncoding.DecodeString(update.SignatureBase64)
	if err != nil {
		return fmt.Errorf("%w: invalid update signature encoding", ErrTrustUpdateSignatureInvalid)
	}
	if !ed25519.Verify(pub, msg, sig) {
		return ErrTrustUpdateSignatureInvalid
	}
	return nil
}

func canonicalBundlePayload(b Bundle) ([]byte, error) {
	return json.Marshal(b)
}

func verifyRootContinuity(current, next Bundle) error {
	for _, nextRoot := range next.RootKeys {
		for _, curRoot := range current.RootKeys {
			if nextRoot.KeyID == curRoot.KeyID && nextRoot.PublicKeyBase64 == curRoot.PublicKeyBase64 {
				return nil
			}
		}
	}
	return ErrTrustUpdateChainInvalid
}

func verifyBundleMonotonic(current, next Bundle) error {
	if next.Version < current.Version {
		return ErrTrustUpdateChainInvalid
	}
	if next.Version == current.Version && next.BundleID != current.BundleID {
		return ErrTrustUpdateChainInvalid
	}
	return nil
}

func verifyActiveManifestKeys(bundle Bundle, at time.Time) error {
	if len(bundle.ActiveManifestKeys(at)) == 0 {
		return ErrTrustUpdateChainInvalid
	}
	return nil
}

func verifyRotationOverlap(current, next Bundle, at time.Time) error {
	currentActive := current.ActiveManifestKeys(at)
	nextActive := next.ActiveManifestKeys(at)
	if len(currentActive) == 0 || len(nextActive) == 0 {
		return ErrTrustUpdateChainInvalid
	}
	nextSet := make(map[string]struct{}, len(nextActive))
	for _, k := range nextActive {
		nextSet[k.KeyID] = struct{}{}
	}
	for _, k := range currentActive {
		if _, ok := nextSet[k.KeyID]; ok {
			return nil
		}
	}
	return ErrTrustUpdateChainInvalid
}
