package main

import (
	"aim-chat/go-backend/internal/bootstrap/networkmanifest"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type reconnectPolicy struct {
	BaseIntervalMS int     `json:"base_interval_ms"`
	MaxIntervalMS  int     `json:"max_interval_ms"`
	JitterRatio    float64 `json:"jitter_ratio"`
}

type manifest struct {
	Version         int             `json:"version"`
	GeneratedAt     time.Time       `json:"generated_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	BootstrapNodes  []string        `json:"bootstrap_nodes"`
	MinPeers        int             `json:"min_peers"`
	ReconnectPolicy reconnectPolicy `json:"reconnect_policy"`
	KeyID           string          `json:"key_id"`
	Signature       string          `json:"signature"`
}

type rootKey struct {
	KeyID           string `json:"key_id"`
	Algorithm       string `json:"algorithm"`
	PublicKeyBase64 string `json:"public_key_base64"`
}

type manifestKey struct {
	KeyID           string    `json:"key_id"`
	Algorithm       string    `json:"algorithm"`
	PublicKeyBase64 string    `json:"public_key_base64"`
	NotBefore       time.Time `json:"not_before"`
	NotAfter        time.Time `json:"not_after"`
}

type trustBundle struct {
	Version      int           `json:"version"`
	BundleID     string        `json:"bundle_id"`
	GeneratedAt  time.Time     `json:"generated_at"`
	RootKeys     []rootKey     `json:"root_keys"`
	ManifestKeys []manifestKey `json:"manifest_keys"`
}

func main() {
	var (
		outDir         = flag.String("out-dir", "", "output directory")
		bootstrapNodes = flag.String("bootstrap-nodes", "", "comma-separated bootstrap multiaddrs")
		version        = flag.Int("version", 1, "manifest version")
		minPeers       = flag.Int("min-peers", 2, "minimum peers")
		baseMS         = flag.Int("base-interval-ms", 1000, "reconnect base interval ms")
		maxMS          = flag.Int("max-interval-ms", 30000, "reconnect max interval ms")
		jitter         = flag.Float64("jitter-ratio", 0.2, "reconnect jitter ratio")
		validFor       = flag.Duration("valid-for", 24*time.Hour, "manifest/trust validity duration")
		keyID          = flag.String("key-id", "manifest-local-docker", "manifest key id")
	)
	flag.Parse()

	if strings.TrimSpace(*outDir) == "" {
		fail("out-dir is required")
	}

	nodes := splitCSV(*bootstrapNodes)
	if len(nodes) == 0 {
		fail("bootstrap-nodes is required")
	}

	now := time.Now().UTC()
	expiry := now.Add(*validFor)
	if !expiry.After(now) {
		fail("valid-for must be > 0")
	}

	rootPub, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		failf("generate root key: %v", err)
	}
	manifestPub, manifestPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		failf("generate manifest key: %v", err)
	}

	tb := trustBundle{
		Version:     1,
		BundleID:    fmt.Sprintf("tb_local_%d", now.Unix()),
		GeneratedAt: now,
		RootKeys: []rootKey{
			{
				KeyID:           "root-local-1",
				Algorithm:       "ed25519",
				PublicKeyBase64: base64.StdEncoding.EncodeToString(rootPub),
			},
		},
		ManifestKeys: []manifestKey{
			{
				KeyID:           strings.TrimSpace(*keyID),
				Algorithm:       "ed25519",
				PublicKeyBase64: base64.StdEncoding.EncodeToString(manifestPub),
				NotBefore:       now.Add(-5 * time.Minute),
				NotAfter:        expiry,
			},
		},
	}

	m := manifest{
		Version:        *version,
		GeneratedAt:    now,
		ExpiresAt:      expiry,
		BootstrapNodes: nodes,
		MinPeers:       *minPeers,
		ReconnectPolicy: reconnectPolicy{
			BaseIntervalMS: *baseMS,
			MaxIntervalMS:  *maxMS,
			JitterRatio:    *jitter,
		},
		KeyID: strings.TrimSpace(*keyID),
	}

	payload, err := networkmanifest.CanonicalPayload(networkmanifest.Manifest{
		Version:        m.Version,
		GeneratedAt:    m.GeneratedAt,
		ExpiresAt:      m.ExpiresAt,
		BootstrapNodes: m.BootstrapNodes,
		MinPeers:       m.MinPeers,
		ReconnectPolicy: networkmanifest.ReconnectPolicy{
			BaseIntervalMS: m.ReconnectPolicy.BaseIntervalMS,
			MaxIntervalMS:  m.ReconnectPolicy.MaxIntervalMS,
			JitterRatio:    m.ReconnectPolicy.JitterRatio,
		},
		KeyID: m.KeyID,
	})
	if err != nil {
		failf("build canonical payload: %v", err)
	}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(manifestPriv, payload))

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		failf("create out dir: %v", err)
	}

	manifestPath := filepath.Join(*outDir, "network-manifest.json")
	trustPath := filepath.Join(*outDir, "trust_bundle.json")
	rootPrivPath := filepath.Join(*outDir, "root_key.private.b64")
	manifestPrivPath := filepath.Join(*outDir, "manifest_key.private.b64")

	writeJSON(manifestPath, m)
	writeJSON(trustPath, tb)
	writeText(rootPrivPath, base64.StdEncoding.EncodeToString(rootPriv))
	writeText(manifestPrivPath, base64.StdEncoding.EncodeToString(manifestPriv))

	writeStdoutln("Generated local manifest bundle:")
	writeStdoutf("  %s\n", manifestPath)
	writeStdoutf("  %s\n", trustPath)
	writeStdoutf("  %s\n", rootPrivPath)
	writeStdoutf("  %s\n", manifestPrivPath)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func writeJSON(path string, value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		failf("marshal json %s: %v", path, err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		failf("write file %s: %v", path, err)
	}
}

func writeText(path, value string) {
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		failf("write file %s: %v", path, err)
	}
}

func fail(msg string) {
	if _, err := fmt.Fprintln(os.Stderr, msg); err != nil {
		os.Exit(1)
	}
	os.Exit(1)
}

func failf(format string, args ...any) {
	if _, err := fmt.Fprintf(os.Stderr, format+"\n", args...); err != nil {
		os.Exit(1)
	}
	os.Exit(1)
}

func writeStdoutln(line string) {
	if _, err := fmt.Fprintln(os.Stdout, line); err != nil {
		os.Exit(1)
	}
}

func writeStdoutf(format string, args ...any) {
	if _, err := fmt.Fprintf(os.Stdout, format, args...); err != nil {
		os.Exit(1)
	}
}
