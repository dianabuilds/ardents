package bootstrapmanager

import (
	"aim-chat/go-backend/internal/bootstrap/manifesttrust"
	"aim-chat/go-backend/internal/waku"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type kp struct {
	pub ed25519.PublicKey
	prv ed25519.PrivateKey
}

func mustKP(t *testing.T) kp {
	t.Helper()
	pub, prv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	return kp{pub: pub, prv: prv}
}

func writeTrustBundle(t *testing.T, path string, now time.Time, root kp, manifestKeyID string, manifestSigner kp) {
	t.Helper()
	bundle := manifesttrust.Bundle{
		Version:     1,
		BundleID:    "tb-1",
		GeneratedAt: now,
		RootKeys: []manifesttrust.RootKey{
			{KeyID: "root-1", Algorithm: "ed25519", PublicKeyBase64: base64.StdEncoding.EncodeToString(root.pub)},
		},
		ManifestKeys: []manifesttrust.ManifestKey{
			{
				KeyID:           manifestKeyID,
				Algorithm:       "ed25519",
				PublicKeyBase64: base64.StdEncoding.EncodeToString(manifestSigner.pub),
				NotBefore:       now.Add(-1 * time.Hour),
				NotAfter:        now.Add(24 * time.Hour),
			},
		},
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal trust bundle: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write trust bundle: %v", err)
	}
}

func writeManifest(t *testing.T, path string, now time.Time, version int, keyID string, signer kp, mutate func(map[string]any)) {
	t.Helper()
	payload := map[string]any{
		"version":         version,
		"generated_at":    now.Add(-1 * time.Minute).UTC().Format(time.RFC3339Nano),
		"expires_at":      now.Add(30 * time.Minute).UTC().Format(time.RFC3339Nano),
		"bootstrap_nodes": []string{"/dns4/bootstrap-1.ardents.net/tcp/60000/p2p/16Uiu2HAmExample"},
		"min_peers":       2,
		"reconnect_policy": map[string]any{
			"base_interval_ms": 1000,
			"max_interval_ms":  30000,
			"jitter_ratio":     0.2,
		},
		"key_id": keyID,
	}
	if mutate != nil {
		mutate(payload)
	}

	signable := map[string]any{}
	for k, v := range payload {
		signable[k] = v
	}
	canonical, err := json.Marshal(signable)
	if err != nil {
		t.Fatalf("marshal signable: %v", err)
	}
	sig := ed25519.Sign(signer.prv, canonical)
	payload["signature"] = base64.StdEncoding.EncodeToString(sig)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func writeCacheSet(t *testing.T, path string, set BootstrapSet) {
	t.Helper()
	payload := cachePayload{
		CachedAt:     time.Now().UTC(),
		SourceOrigin: SourceManifest,
		Set:          set,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal cache payload: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write cache payload: %v", err)
	}
}

func bakedSet() BootstrapSet {
	return BootstrapSet{
		Source:         SourceBaked,
		BootstrapNodes: []string{"/dns4/baked-1.ardents.net/tcp/60000/p2p/16Uiu2HAmBaked"},
		MinPeers:       1,
		ReconnectPolicy: ReconnectPolicy{
			BaseIntervalMS: 1000,
			MaxIntervalMS:  30000,
			JitterRatio:    0.2,
		},
	}
}

func TestLoadPrefersManifestOverCacheAndBaked(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	manifestPath := filepath.Join(tmp, "manifest.json")
	trustPath := filepath.Join(tmp, "trust_bundle.json")
	cachePath := filepath.Join(tmp, "cache", "bootstrap-cache.json")
	root := mustKP(t)
	signer := mustKP(t)
	writeTrustBundle(t, trustPath, now, root, "manifest-2026-q1", signer)
	writeManifest(t, manifestPath, now, 12, "manifest-2026-q1", signer, nil)
	writeCacheSet(t, cachePath, bakedSet())

	mgr := New(manifestPath, trustPath, cachePath, bakedSet())
	mgr.now = func() time.Time { return now }
	res := mgr.LoadBootstrapSet()
	if !res.OK || res.Set == nil {
		t.Fatalf("expected load success from manifest, got %+v", res)
	}
	if res.Set.Source != SourceManifest {
		t.Fatalf("expected source manifest, got %s", res.Set.Source)
	}
}

func TestLoadFallsBackToCacheWhenManifestInvalid(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	manifestPath := filepath.Join(tmp, "manifest.json")
	trustPath := filepath.Join(tmp, "trust_bundle.json")
	cachePath := filepath.Join(tmp, "cache", "bootstrap-cache.json")
	root := mustKP(t)
	signer := mustKP(t)
	writeTrustBundle(t, trustPath, now, root, "manifest-2026-q1", signer)
	writeManifest(t, manifestPath, now, 12, "manifest-2026-q1", signer, func(v map[string]any) {
		v["bootstrap_nodes"] = []string{}
	})

	cacheSet := BootstrapSet{
		Source:         SourceCache,
		BootstrapNodes: []string{"/dns4/cache-1.ardents.net/tcp/60000/p2p/16Uiu2HAmCache"},
		MinPeers:       2,
		ReconnectPolicy: ReconnectPolicy{
			BaseIntervalMS: 1000,
			MaxIntervalMS:  30000,
			JitterRatio:    0.2,
		},
	}
	writeCacheSet(t, cachePath, cacheSet)

	mgr := New(manifestPath, trustPath, cachePath, bakedSet())
	mgr.now = func() time.Time { return now }
	res := mgr.LoadBootstrapSet()
	if !res.OK || res.Set == nil {
		t.Fatalf("expected load success from cache, got %+v", res)
	}
	if res.Set.Source != SourceCache {
		t.Fatalf("expected source cache, got %s", res.Set.Source)
	}
}

func TestLoadFallsBackToBakedWhenManifestAndCacheInvalid(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	manifestPath := filepath.Join(tmp, "manifest.json")
	trustPath := filepath.Join(tmp, "trust_bundle.json")
	cachePath := filepath.Join(tmp, "cache", "bootstrap-cache.json")
	root := mustKP(t)
	signer := mustKP(t)
	writeTrustBundle(t, trustPath, now, root, "manifest-2026-q1", signer)
	writeManifest(t, manifestPath, now, 12, "manifest-2026-q1", signer, func(v map[string]any) {
		v["reconnect_policy"] = map[string]any{
			"base_interval_ms": 2000,
			"max_interval_ms":  1000,
			"jitter_ratio":     0.2,
		}
	})
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte(`{"broken":`), 0o644); err != nil {
		t.Fatalf("write invalid cache: %v", err)
	}

	mgr := New(manifestPath, trustPath, cachePath, bakedSet())
	mgr.now = func() time.Time { return now }
	res := mgr.LoadBootstrapSet()
	if !res.OK || res.Set == nil {
		t.Fatalf("expected load success from baked, got %+v", res)
	}
	if res.Set.Source != SourceBaked {
		t.Fatalf("expected source baked, got %s", res.Set.Source)
	}
}

func TestApplySetValidatesPolicy(t *testing.T) {
	cfg := waku.DefaultConfig()
	mgr := New("", "", "", bakedSet())
	invalid := bakedSet()
	invalid.ReconnectPolicy.BaseIntervalMS = 100
	result := mgr.ApplyBootstrapSet(&cfg, invalid)
	if result.Applied {
		t.Fatalf("expected apply rejected, got %+v", result)
	}
	if result.ErrorCode != "BOOTSTRAP_SET_INVALID" {
		t.Fatalf("expected BOOTSTRAP_SET_INVALID, got %s", result.ErrorCode)
	}
}

func TestApplySetIsIdempotentForSameSet(t *testing.T) {
	cfg := waku.DefaultConfig()
	mgr := New("", "", "", bakedSet())
	set := bakedSet()

	first := mgr.ApplyBootstrapSet(&cfg, set)
	second := mgr.ApplyBootstrapSet(&cfg, set)
	if !first.Applied || !second.Applied {
		t.Fatalf("expected idempotent apply success, got first=%+v second=%+v", first, second)
	}
	if cfg.BootstrapSource != SourceBaked {
		t.Fatalf("expected bootstrap source baked, got %s", cfg.BootstrapSource)
	}
}

func TestLoadFailsWhenAllSourcesUnavailable(t *testing.T) {
	tmp := t.TempDir()
	mgr := New(filepath.Join(tmp, "manifest.json"), filepath.Join(tmp, "trust_bundle.json"), filepath.Join(tmp, "cache", "bootstrap-cache.json"), BootstrapSet{})
	res := mgr.LoadBootstrapSet()
	if res.OK {
		t.Fatalf("expected load failure, got %+v", res)
	}
	if res.ErrorCode != "BOOTSTRAP_SET_UNAVAILABLE" {
		t.Fatalf("expected BOOTSTRAP_SET_UNAVAILABLE, got %s", res.ErrorCode)
	}
}

func TestLoadBlocksDowngradeAttackAndUsesCache(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	manifestPath := filepath.Join(tmp, "manifest.json")
	trustPath := filepath.Join(tmp, "trust_bundle.json")
	cachePath := filepath.Join(tmp, "cache", "bootstrap-cache.json")
	root := mustKP(t)
	signer := mustKP(t)

	writeTrustBundle(t, trustPath, now, root, "manifest-2026-q1", signer)
	writeManifest(t, manifestPath, now, 9, "manifest-2026-q1", signer, nil)
	writeCacheSet(t, cachePath, BootstrapSet{
		Source:         SourceManifest,
		BootstrapNodes: []string{"/dns4/cache-1.ardents.net/tcp/60000/p2p/16Uiu2HAmCache"},
		MinPeers:       2,
		ReconnectPolicy: ReconnectPolicy{
			BaseIntervalMS: 1000,
			MaxIntervalMS:  30000,
			JitterRatio:    0.2,
		},
		ManifestMeta: &ManifestMeta{
			Version:     10,
			GeneratedAt: now.Add(-2 * time.Minute),
			ExpiresAt:   now.Add(30 * time.Minute),
			KeyID:       "manifest-2026-q1",
			FetchedAt:   now.Add(-1 * time.Minute),
		},
	})

	mgr := New(manifestPath, trustPath, cachePath, bakedSet())
	mgr.now = func() time.Time { return now }
	res := mgr.LoadBootstrapSet()
	if !res.OK || res.Set == nil {
		t.Fatalf("expected load success via cache fallback, got %+v", res)
	}
	if res.Set.Source != SourceCache {
		t.Fatalf("expected cache fallback, got %s", res.Set.Source)
	}
	if mgr.LastRejectCode() != "MANIFEST_REPLAY_DETECTED" {
		t.Fatalf("expected MANIFEST_REPLAY_DETECTED, got %q", mgr.LastRejectCode())
	}
}

func TestLoadBlocksMissingTrustChain(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	manifestPath := filepath.Join(tmp, "manifest.json")
	trustPath := filepath.Join(tmp, "trust_bundle.json")
	root := mustKP(t)
	signer := mustKP(t)

	// Manifest exists, trust bundle missing -> trust chain invalid must block manifest source.
	writeManifest(t, manifestPath, now, 12, "manifest-2026-q1", signer, nil)
	_ = root

	mgr := New(manifestPath, trustPath, "", bakedSet())
	mgr.now = func() time.Time { return now }
	res := mgr.LoadBootstrapSet()
	if !res.OK || res.Set == nil {
		t.Fatalf("expected fallback to baked, got %+v", res)
	}
	if res.Set.Source != SourceBaked {
		t.Fatalf("expected baked source, got %s", res.Set.Source)
	}
	if mgr.LastRejectCode() != "TRUST_BUNDLE_INVALID" {
		t.Fatalf("expected TRUST_BUNDLE_INVALID, got %q", mgr.LastRejectCode())
	}
}
