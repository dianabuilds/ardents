package manifesttrust

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type keyPair struct {
	pub ed25519.PublicKey
	prv ed25519.PrivateKey
}

func mustKeyPair(t *testing.T) keyPair {
	t.Helper()
	pub, prv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	return keyPair{pub: pub, prv: prv}
}

func b64(v []byte) string {
	return base64.StdEncoding.EncodeToString(v)
}

func makeBundle(now time.Time, root keyPair, manifestKeys map[string]keyPair, version int, bundleID string) Bundle {
	keys := make([]ManifestKey, 0, len(manifestKeys))
	for id, kp := range manifestKeys {
		keys = append(keys, ManifestKey{
			KeyID:           id,
			Algorithm:       "ed25519",
			PublicKeyBase64: b64(kp.pub),
			NotBefore:       now.Add(-1 * time.Hour),
			NotAfter:        now.Add(24 * time.Hour),
		})
	}
	return Bundle{
		Version:     version,
		BundleID:    bundleID,
		GeneratedAt: now,
		RootKeys: []RootKey{
			{
				KeyID:           "root-1",
				Algorithm:       "ed25519",
				PublicKeyBase64: b64(root.pub),
			},
		},
		ManifestKeys: keys,
	}
}

func signUpdate(t *testing.T, bundle Bundle, signerKeyID string, signer ed25519.PrivateKey) BundleUpdateEnvelope {
	t.Helper()
	payload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	sig := ed25519.Sign(signer, payload)
	return BundleUpdateEnvelope{
		Bundle:          bundle,
		SignedByKeyID:   signerKeyID,
		SignatureBase64: b64(sig),
	}
}

func TestVerifyManifestSignature(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustKeyPair(t)
	manifestKey := mustKeyPair(t)
	bundle := makeBundle(now, root, map[string]keyPair{"manifest-a": manifestKey}, 1, "tb-1")

	payload := []byte("manifest-payload")
	signature := ed25519.Sign(manifestKey.prv, payload)

	if err := VerifyManifestSignature(bundle, "manifest-a", payload, signature, now); err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}
	if err := VerifyManifestSignature(bundle, "manifest-unknown", payload, signature, now); !errors.Is(err, ErrManifestKeyUnknown) {
		t.Fatalf("expected ErrManifestKeyUnknown, got %v", err)
	}
}

func TestVerifyAndApplyUpdateRejectsUnknownRootSigner(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustKeyPair(t)
	oldManifest := mustKeyPair(t)
	current := makeBundle(now, root, map[string]keyPair{"manifest-old": oldManifest}, 1, "tb-1")

	next := makeBundle(now, root, map[string]keyPair{"manifest-old": oldManifest}, 2, "tb-2")
	env := signUpdate(t, next, "root-unknown", root.prv)

	_, err := VerifyAndApplyUpdate(current, env, now)
	if !errors.Is(err, ErrTrustUpdateChainInvalid) {
		t.Fatalf("expected ErrTrustUpdateChainInvalid, got %v", err)
	}
}

func TestVerifyAndApplyUpdateRejectsInvalidSignature(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustKeyPair(t)
	rootOther := mustKeyPair(t)
	oldManifest := mustKeyPair(t)
	current := makeBundle(now, root, map[string]keyPair{"manifest-old": oldManifest}, 1, "tb-1")

	next := makeBundle(now, root, map[string]keyPair{"manifest-old": oldManifest}, 2, "tb-2")
	env := signUpdate(t, next, "root-1", rootOther.prv)

	_, err := VerifyAndApplyUpdate(current, env, now)
	if !errors.Is(err, ErrTrustUpdateSignatureInvalid) {
		t.Fatalf("expected ErrTrustUpdateSignatureInvalid, got %v", err)
	}
}

func TestVerifyAndApplyUpdateAcceptsOverlapRotation(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustKeyPair(t)
	oldManifest := mustKeyPair(t)
	newManifest := mustKeyPair(t)

	current := makeBundle(now, root, map[string]keyPair{"manifest-old": oldManifest}, 1, "tb-1")
	next := makeBundle(now, root, map[string]keyPair{
		"manifest-old": oldManifest,
		"manifest-new": newManifest,
	}, 2, "tb-2")
	env := signUpdate(t, next, "root-1", root.prv)

	applied, err := VerifyAndApplyUpdate(current, env, now)
	if err != nil {
		t.Fatalf("expected overlap update accepted, got %v", err)
	}
	if err := VerifyManifestSignature(applied, "manifest-old", []byte("x"), ed25519.Sign(oldManifest.prv, []byte("x")), now); err != nil {
		t.Fatalf("old key must remain valid during overlap, got %v", err)
	}
	if err := VerifyManifestSignature(applied, "manifest-new", []byte("y"), ed25519.Sign(newManifest.prv, []byte("y")), now); err != nil {
		t.Fatalf("new key must be valid after update, got %v", err)
	}
}

func TestVerifyAndApplyUpdateRejectsNoOverlapRotation(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustKeyPair(t)
	oldManifest := mustKeyPair(t)
	newManifest := mustKeyPair(t)

	current := makeBundle(now, root, map[string]keyPair{"manifest-old": oldManifest}, 1, "tb-1")
	next := makeBundle(now, root, map[string]keyPair{"manifest-new": newManifest}, 2, "tb-2")
	env := signUpdate(t, next, "root-1", root.prv)

	_, err := VerifyAndApplyUpdate(current, env, now)
	if !errors.Is(err, ErrTrustUpdateChainInvalid) {
		t.Fatalf("expected ErrTrustUpdateChainInvalid for no-overlap rotation, got %v", err)
	}
}
