package bootstrapmanager

import (
	"aim-chat/go-backend/internal/bootstrap/manifesttrust"
	"aim-chat/go-backend/internal/bootstrap/networkmanifest"
	"aim-chat/go-backend/internal/waku"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	SourceManifest = "manifest"
	SourceCache    = "cache"
	SourceBaked    = "baked"
)

type ReconnectPolicy struct {
	BaseIntervalMS int     `json:"base_interval_ms"`
	MaxIntervalMS  int     `json:"max_interval_ms"`
	JitterRatio    float64 `json:"jitter_ratio"`
}

type ManifestMeta struct {
	Version     int       `json:"version"`
	GeneratedAt time.Time `json:"generated_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	KeyID       string    `json:"key_id"`
	FetchedAt   time.Time `json:"fetched_at"`
}

type BootstrapSet struct {
	Source          string          `json:"source"`
	BootstrapNodes  []string        `json:"bootstrap_nodes"`
	MinPeers        int             `json:"min_peers"`
	ReconnectPolicy ReconnectPolicy `json:"reconnect_policy"`
	ManifestMeta    *ManifestMeta   `json:"manifest_meta,omitempty"`
}

type LoadResult struct {
	OK        bool          `json:"ok"`
	Set       *BootstrapSet `json:"set,omitempty"`
	ErrorCode string        `json:"error_code,omitempty"`
	Reason    string        `json:"reason,omitempty"`
}

type ApplyResult struct {
	Applied   bool   `json:"applied"`
	ErrorCode string `json:"error_code,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type cachePayload struct {
	CachedAt     time.Time    `json:"cached_at"`
	SourceOrigin string       `json:"source_origin"`
	Set          BootstrapSet `json:"set"`
}

type Manager struct {
	manifestPath    string
	trustBundlePath string
	cachePath       string
	now             func() time.Time

	baked BootstrapSet

	activeSource   string
	manifestMeta   *ManifestMeta
	lastReason     string
	lastRejectCode string
}

func New(manifestPath, trustBundlePath, cachePath string, baked BootstrapSet) *Manager {
	return &Manager{
		manifestPath:    manifestPath,
		trustBundlePath: trustBundlePath,
		cachePath:       cachePath,
		now:             func() time.Time { return time.Now().UTC() },
		baked:           baked,
		activeSource:    SourceBaked,
	}
}

func (m *Manager) LoadBootstrapSet() LoadResult {
	now := m.now()

	manifestSet, manifestErr := m.loadManifest(now)
	if manifestErr == nil {
		m.lastRejectCode = ""
		return LoadResult{OK: true, Set: manifestSet}
	}
	m.lastReason = fmt.Sprintf("manifest rejected: %v", manifestErr)
	m.lastRejectCode = mapManifestRejectCode(manifestErr)

	cacheSet, cacheErr := m.loadCache()
	if cacheErr == nil {
		return LoadResult{OK: true, Set: cacheSet}
	}
	m.lastReason = fmt.Sprintf("%s; cache invalid: %v", m.lastReason, cacheErr)

	if err := validateSet(m.baked); err == nil {
		set := m.baked
		set.Source = SourceBaked
		return LoadResult{OK: true, Set: &set}
	}

	return LoadResult{
		OK:        false,
		ErrorCode: "BOOTSTRAP_SET_UNAVAILABLE",
		Reason:    m.lastReason,
	}
}

func (m *Manager) ApplyBootstrapSet(cfg *waku.Config, set BootstrapSet) ApplyResult {
	if cfg == nil {
		return ApplyResult{Applied: false, ErrorCode: "BOOTSTRAP_APPLY_FAILED", Reason: "nil config"}
	}
	if err := validateSet(set); err != nil {
		return ApplyResult{Applied: false, ErrorCode: "BOOTSTRAP_SET_INVALID", Reason: err.Error()}
	}

	cfg.BootstrapNodes = append([]string(nil), set.BootstrapNodes...)
	cfg.MinPeers = set.MinPeers
	cfg.ReconnectInterval = time.Duration(set.ReconnectPolicy.BaseIntervalMS) * time.Millisecond
	cfg.ReconnectBackoffMax = time.Duration(set.ReconnectPolicy.MaxIntervalMS) * time.Millisecond
	cfg.BootstrapSource = set.Source
	if set.ManifestMeta != nil {
		cfg.BootstrapManifestVersion = set.ManifestMeta.Version
		cfg.BootstrapManifestKeyID = set.ManifestMeta.KeyID
	} else {
		cfg.BootstrapManifestVersion = 0
		cfg.BootstrapManifestKeyID = ""
	}

	m.activeSource = set.Source
	m.manifestMeta = set.ManifestMeta

	if set.Source == SourceManifest {
		if err := m.saveCache(set); err != nil {
			return ApplyResult{Applied: false, ErrorCode: "BOOTSTRAP_APPLY_FAILED", Reason: err.Error()}
		}
	}
	return ApplyResult{Applied: true}
}

func (m *Manager) GetBootstrapSource() string {
	return m.activeSource
}

func (m *Manager) GetManifestMeta() *ManifestMeta {
	if m.manifestMeta == nil {
		return nil
	}
	copyMeta := *m.manifestMeta
	return &copyMeta
}

func (m *Manager) LastReason() string {
	return m.lastReason
}

func (m *Manager) LastRejectCode() string {
	return m.lastRejectCode
}

func (m *Manager) loadManifest(now time.Time) (*BootstrapSet, error) {
	if m.manifestPath == "" || m.trustBundlePath == "" {
		return nil, errors.New("manifest or trust bundle path is not configured")
	}
	manifestRaw, err := os.ReadFile(m.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("manifest load failed: %w", err)
	}
	trustRaw, err := os.ReadFile(m.trustBundlePath)
	if err != nil {
		return nil, fmt.Errorf("trust bundle load failed: %w", err)
	}
	trustBundle, err := manifesttrust.ParseBundle(trustRaw)
	if err != nil {
		return nil, err
	}

	lastVersion := m.readCachedManifestVersion()
	verified, err := networkmanifest.Verify(networkmanifest.VerifyRequest{
		Raw:                manifestRaw,
		TrustBundle:        trustBundle,
		Now:                now,
		LastAppliedVersion: lastVersion,
	})
	if err != nil {
		return nil, err
	}
	set := BootstrapSet{
		Source:         SourceManifest,
		BootstrapNodes: append([]string(nil), verified.BootstrapNodes...),
		MinPeers:       verified.MinPeers,
		ReconnectPolicy: ReconnectPolicy{
			BaseIntervalMS: verified.ReconnectPolicy.BaseIntervalMS,
			MaxIntervalMS:  verified.ReconnectPolicy.MaxIntervalMS,
			JitterRatio:    verified.ReconnectPolicy.JitterRatio,
		},
		ManifestMeta: &ManifestMeta{
			Version:     verified.Version,
			GeneratedAt: verified.GeneratedAt,
			ExpiresAt:   verified.ExpiresAt,
			KeyID:       verified.KeyID,
			FetchedAt:   now,
		},
	}
	if err := validateSet(set); err != nil {
		return nil, err
	}
	return &set, nil
}

func (m *Manager) loadCache() (*BootstrapSet, error) {
	if m.cachePath == "" {
		return nil, errors.New("cache path is not configured")
	}
	raw, err := os.ReadFile(m.cachePath)
	if err != nil {
		return nil, err
	}
	var cached cachePayload
	if err := json.Unmarshal(raw, &cached); err != nil {
		return nil, err
	}
	if err := validateSet(cached.Set); err != nil {
		return nil, err
	}
	cached.Set.Source = SourceCache
	return &cached.Set, nil
}

func (m *Manager) saveCache(set BootstrapSet) error {
	if m.cachePath == "" {
		return nil
	}
	payload := cachePayload{
		CachedAt:     m.now(),
		SourceOrigin: SourceManifest,
		Set:          set,
	}
	payload.Set.Source = SourceManifest
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.cachePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.cachePath, raw, 0o644)
}

func (m *Manager) readCachedManifestVersion() int {
	if m.cachePath == "" {
		return 0
	}
	raw, err := os.ReadFile(m.cachePath)
	if err != nil {
		return 0
	}
	var cached cachePayload
	if err := json.Unmarshal(raw, &cached); err != nil {
		return 0
	}
	if cached.Set.ManifestMeta == nil {
		return 0
	}
	return cached.Set.ManifestMeta.Version
}

func validateSet(set BootstrapSet) error {
	if len(set.BootstrapNodes) < 1 {
		return errors.New("bootstrap_nodes must contain at least one node")
	}
	if set.MinPeers < 1 {
		return errors.New("min_peers must be >= 1")
	}
	if set.ReconnectPolicy.BaseIntervalMS < 500 {
		return errors.New("base_interval_ms must be >= 500")
	}
	if set.ReconnectPolicy.MaxIntervalMS < set.ReconnectPolicy.BaseIntervalMS {
		return errors.New("max_interval_ms must be >= base_interval_ms")
	}
	if set.ReconnectPolicy.JitterRatio < 0 || set.ReconnectPolicy.JitterRatio > 1 {
		return errors.New("jitter_ratio must be in [0..1]")
	}
	return nil
}

func mapManifestRejectCode(err error) string {
	if err == nil {
		return ""
	}
	if code, ok := networkmanifest.RejectCodeOf(err); ok {
		return string(code)
	}
	if errors.Is(err, manifesttrust.ErrTrustBundleInvalid) || errors.Is(err, manifesttrust.ErrTrustUpdateChainInvalid) {
		return "TRUST_BUNDLE_INVALID"
	}
	reason := strings.ToLower(err.Error())
	if strings.Contains(reason, "trust bundle") {
		return "TRUST_BUNDLE_INVALID"
	}
	return ""
}
