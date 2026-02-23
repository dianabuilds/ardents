package networkmanifest

import (
	"aim-chat/go-backend/internal/bootstrap/manifesttrust"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

type testKeyPair struct {
	pub ed25519.PublicKey
	prv ed25519.PrivateKey
}

func mustTestKeyPair(t *testing.T) testKeyPair {
	t.Helper()
	pub, prv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	return testKeyPair{pub: pub, prv: prv}
}

func buildTrustBundle(now time.Time, root testKeyPair, manifestID string, manifestKey testKeyPair) manifesttrust.Bundle {
	return manifesttrust.Bundle{
		Version:     1,
		BundleID:    "tb-1",
		GeneratedAt: now,
		RootKeys: []manifesttrust.RootKey{
			{
				KeyID:           "root-1",
				Algorithm:       "ed25519",
				PublicKeyBase64: base64.StdEncoding.EncodeToString(root.pub),
			},
		},
		ManifestKeys: []manifesttrust.ManifestKey{
			{
				KeyID:           manifestID,
				Algorithm:       "ed25519",
				PublicKeyBase64: base64.StdEncoding.EncodeToString(manifestKey.pub),
				NotBefore:       now.Add(-1 * time.Hour),
				NotAfter:        now.Add(24 * time.Hour),
			},
		},
	}
}

func buildManifest(now time.Time, keyID string) Manifest {
	return Manifest{
		Version:        12,
		GeneratedAt:    now,
		ExpiresAt:      now.Add(30 * time.Minute),
		BootstrapNodes: []string{"/dns4/bootstrap-1.ardents.net/tcp/60000/p2p/16Uiu2HAmExample"},
		MinPeers:       2,
		ReconnectPolicy: ReconnectPolicy{
			BaseIntervalMS: 1000,
			MaxIntervalMS:  30000,
			JitterRatio:    0.2,
		},
		KeyID: keyID,
	}
}

func mustSignManifest(t *testing.T, m Manifest, prv ed25519.PrivateKey) Manifest {
	t.Helper()
	payload, err := canonicalPayload(m)
	if err != nil {
		t.Fatalf("canonical payload: %v", err)
	}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(prv, payload))
	return m
}

func mustRaw(t *testing.T, m Manifest) []byte {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return raw
}

func verifyCode(t *testing.T, err error, expected RejectCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error, got nil", expected)
	}
	verr, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("expected VerifyError, got %T (%v)", err, err)
	}
	if verr.Code != expected {
		t.Fatalf("expected reject code %s, got %s (%v)", expected, verr.Code, err)
	}
}

func TestVerifyValidManifestAccepted(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustTestKeyPair(t)
	manifestSigner := mustTestKeyPair(t)
	bundle := buildTrustBundle(now, root, "manifest-2026-q1", manifestSigner)
	manifest := mustSignManifest(t, buildManifest(now, "manifest-2026-q1"), manifestSigner.prv)

	verified, err := Verify(VerifyRequest{
		Raw:         mustRaw(t, manifest),
		TrustBundle: bundle,
		Now:         now.Add(1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("expected manifest accepted, got %v", err)
	}
	if verified.Version != manifest.Version || verified.KeyID != manifest.KeyID {
		t.Fatalf("unexpected verified payload: %+v", verified)
	}
}

func TestVerifyMissingFieldRejected(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	raw := []byte(`{"version":12}`)
	_, err := Verify(VerifyRequest{
		Raw:         raw,
		TrustBundle: manifesttrust.Bundle{},
		Now:         now,
	})
	verifyCode(t, err, RejectSchemaInvalid)
}

func TestVerifyPolicyGuardRejected(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustTestKeyPair(t)
	manifestSigner := mustTestKeyPair(t)
	bundle := buildTrustBundle(now, root, "manifest-2026-q1", manifestSigner)
	manifest := buildManifest(now, "manifest-2026-q1")
	manifest.ReconnectPolicy.BaseIntervalMS = 2000
	manifest.ReconnectPolicy.MaxIntervalMS = 1000
	manifest = mustSignManifest(t, manifest, manifestSigner.prv)

	_, err := Verify(VerifyRequest{
		Raw:         mustRaw(t, manifest),
		TrustBundle: bundle,
		Now:         now,
	})
	verifyCode(t, err, RejectPolicyInvalid)
}

func TestVerifyUnknownKeyRejected(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustTestKeyPair(t)
	manifestSigner := mustTestKeyPair(t)
	bundle := buildTrustBundle(now, root, "manifest-known", manifestSigner)
	manifest := mustSignManifest(t, buildManifest(now, "manifest-unknown"), manifestSigner.prv)

	_, err := Verify(VerifyRequest{
		Raw:         mustRaw(t, manifest),
		TrustBundle: bundle,
		Now:         now,
	})
	verifyCode(t, err, RejectKeyUnknown)
}

func TestVerifyInvalidSignatureRejected(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustTestKeyPair(t)
	manifestSigner := mustTestKeyPair(t)
	otherSigner := mustTestKeyPair(t)
	bundle := buildTrustBundle(now, root, "manifest-2026-q1", manifestSigner)
	manifest := mustSignManifest(t, buildManifest(now, "manifest-2026-q1"), otherSigner.prv)

	_, err := Verify(VerifyRequest{
		Raw:         mustRaw(t, manifest),
		TrustBundle: bundle,
		Now:         now,
	})
	verifyCode(t, err, RejectSignatureInvalid)
}

func TestVerifyExpiredManifestRejected(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustTestKeyPair(t)
	manifestSigner := mustTestKeyPair(t)
	bundle := buildTrustBundle(now, root, "manifest-2026-q1", manifestSigner)
	manifest := buildManifest(now, "manifest-2026-q1")
	manifest.GeneratedAt = now.Add(-2 * time.Minute)
	manifest.ExpiresAt = now.Add(-1 * time.Minute)
	manifest = mustSignManifest(t, manifest, manifestSigner.prv)

	_, err := Verify(VerifyRequest{
		Raw:         mustRaw(t, manifest),
		TrustBundle: bundle,
		Now:         now,
	})
	verifyCode(t, err, RejectExpired)
}

func TestVerifyReplayedOlderVersionRejected(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	root := mustTestKeyPair(t)
	manifestSigner := mustTestKeyPair(t)
	bundle := buildTrustBundle(now, root, "manifest-2026-q1", manifestSigner)
	manifest := buildManifest(now, "manifest-2026-q1")
	manifest.Version = 10
	manifest = mustSignManifest(t, manifest, manifestSigner.prv)

	_, err := Verify(VerifyRequest{
		Raw:                mustRaw(t, manifest),
		TrustBundle:        bundle,
		Now:                now,
		LastAppliedVersion: 11,
	})
	verifyCode(t, err, RejectReplay)
}
