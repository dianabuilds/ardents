package bootstrapmanager

import (
	"aim-chat/go-backend/internal/waku"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRefresherFallbackAndRestore(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	manifestPath := filepath.Join(tmp, "manifest.json")
	trustPath := filepath.Join(tmp, "trust_bundle.json")
	cachePath := filepath.Join(tmp, "network", "bootstrap-cache.json")

	rootPub, rootPrv, _ := ed25519.GenerateKey(rand.Reader)
	_ = rootPrv
	signerPub, signerPrv, _ := ed25519.GenerateKey(rand.Reader)
	root := kp{pub: rootPub, prv: rootPrv}
	signer := kp{pub: signerPub, prv: signerPrv}

	writeTrustBundle(t, trustPath, now, root, "manifest-2026-q1", signer)
	writeManifest(t, manifestPath, now, 12, "manifest-2026-q1", signer, nil)

	cfg := waku.DefaultConfig()
	baked := bakedSet()
	mgr := New(manifestPath, trustPath, cachePath, baked)
	mgr.now = func() time.Time { return now }

	ref := NewRefresher(mgr, &cfg, nil)
	first := ref.step(now)
	if cfg.BootstrapSource != SourceManifest {
		t.Fatalf("expected manifest source after first step, got %s", cfg.BootstrapSource)
	}
	if first.SourceAfter != "manifest" {
		t.Fatalf("expected controller source manifest, got %s", first.SourceAfter)
	}

	if err := os.WriteFile(manifestPath, []byte(`{"broken":`), 0o644); err != nil {
		t.Fatalf("break manifest: %v", err)
	}
	second := ref.step(now.Add(2 * time.Second))
	if cfg.BootstrapSource != SourceCache {
		t.Fatalf("expected cache fallback, got %s", cfg.BootstrapSource)
	}
	if second.SourceAfter != "cache" && second.SourceAfter != "baked" {
		t.Fatalf("expected fallback source cache/baked, got %s", second.SourceAfter)
	}

	writeManifest(t, manifestPath, now.Add(3*time.Second), 13, "manifest-2026-q1", signer, nil)
	third := ref.step(now.Add(4 * time.Second))
	if cfg.BootstrapSource != SourceManifest {
		t.Fatalf("expected restore to manifest, got %s", cfg.BootstrapSource)
	}
	if !third.RestoredManifest {
		t.Fatal("expected restored manifest decision")
	}
}
