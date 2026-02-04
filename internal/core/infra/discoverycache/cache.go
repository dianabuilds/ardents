package discoverycache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

var (
	ErrCacheInvalid = errors.New("ERR_CLIENT_DISCOVERY_CACHE_INVALID")
)

type Cache struct {
	V           uint64  `json:"v"`
	UpdatedAtMs int64   `json:"updated_at_ms"`
	Entries     []Entry `json:"entries"`
}

type Entry struct {
	PeerID          string `json:"peer_id"`
	Addr            string `json:"addr"`
	Source          string `json:"source"`
	LastOKMs        int64  `json:"last_ok_ms,omitempty"`
	LastFailMs      int64  `json:"last_fail_ms,omitempty"`
	FailCount       uint64 `json:"fail_count,omitempty"`
	CooldownUntilMs int64  `json:"cooldown_until_ms,omitempty"`
	ExpiresAtMs     int64  `json:"expires_at_ms"`
}

func LoadOrInit(path string) (Cache, error) {
	if path == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			path = d.DiscoveryCachePath()
		} else {
			path = filepath.Join(os.TempDir(), "ardents", "data", "discovery_cache.json")
		}
	}
	if _, err := os.Stat(path); err == nil {
		return Load(path)
	}
	c := Cache{
		V:           1,
		UpdatedAtMs: time.Now().UTC().UnixMilli(),
		Entries:     []Entry{},
	}
	if err := Save(path, c); err != nil {
		return Cache{}, err
	}
	return c, nil
}

func Load(path string) (Cache, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is controlled by app dirs.
	if err != nil {
		return Cache{}, err
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return Cache{}, ErrCacheInvalid
	}
	if c.V != 1 {
		return Cache{}, ErrCacheInvalid
	}
	return c, nil
}

func Save(path string, c Cache) error {
	if path == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			path = d.DiscoveryCachePath()
		} else {
			path = filepath.Join(os.TempDir(), "ardents", "data", "discovery_cache.json")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c Cache) Prune(nowMs int64) Cache {
	out := Cache{V: 1, UpdatedAtMs: nowMs, Entries: make([]Entry, 0, len(c.Entries))}
	for _, e := range c.Entries {
		if e.PeerID == "" || e.Addr == "" {
			continue
		}
		if e.ExpiresAtMs != 0 && nowMs > e.ExpiresAtMs {
			continue
		}
		out.Entries = append(out.Entries, e)
	}
	return out
}

func (c Cache) Candidates(nowMs int64) []Entry {
	pruned := c.Prune(nowMs)
	out := make([]Entry, 0, len(pruned.Entries))
	for _, e := range pruned.Entries {
		if e.CooldownUntilMs != 0 && nowMs < e.CooldownUntilMs {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		// Prefer recently OK.
		if out[i].LastOKMs != out[j].LastOKMs {
			return out[i].LastOKMs > out[j].LastOKMs
		}
		// Then fewer failures.
		if out[i].FailCount != out[j].FailCount {
			return out[i].FailCount < out[j].FailCount
		}
		// Stable.
		if out[i].PeerID != out[j].PeerID {
			return out[i].PeerID < out[j].PeerID
		}
		return out[i].Addr < out[j].Addr
	})
	return out
}

func (c Cache) UpsertBootstrapPeers(peers []config.ClientPeer, nowMs int64, ttlMs int64, lim config.ClientLimits, addLimiter *AddLimiter) Cache {
	out := c.Prune(nowMs)
	for _, p := range peers {
		for _, addr := range p.Addrs {
			out = out.UpsertCandidate(p.PeerID, addr, "bootstrap", nowMs, nowMs+ttlMs, lim, addLimiter)
		}
	}
	return out
}

func (c Cache) MarkOK(peerID string, addr string, nowMs int64, expiresAtMs int64, lim config.ClientLimits) Cache {
	return c.upsertAt(peerID, addr, nowMs, func(e Entry) Entry {
		e.LastOKMs = nowMs
		e.FailCount = 0
		e.CooldownUntilMs = 0
		if expiresAtMs > e.ExpiresAtMs {
			e.ExpiresAtMs = expiresAtMs
		}
		return e
	}, lim)
}

func (c Cache) MarkFail(peerID string, addr string, nowMs int64, lim config.ClientLimits) Cache {
	return c.upsertAt(peerID, addr, nowMs, func(e Entry) Entry {
		e.LastFailMs = nowMs
		e.FailCount++
		e.CooldownUntilMs = nowMs + cooldownMs(e.FailCount, lim)
		return e
	}, lim)
}

func (c Cache) UpsertCandidate(peerID string, addr string, source string, nowMs int64, expiresAtMs int64, lim config.ClientLimits, addLimiter *AddLimiter) Cache {
	if peerID == "" || addr == "" {
		return c
	}
	if addLimiter != nil && !c.has(peerID, addr) {
		if !addLimiter.Allow(nowMs) {
			return c
		}
	}
	return c.upsertAt(peerID, addr, nowMs, func(e Entry) Entry {
		if e.Source == "" {
			e.Source = source
		}
		if expiresAtMs > e.ExpiresAtMs {
			e.ExpiresAtMs = expiresAtMs
		}
		return e
	}, lim)
}

func (c Cache) has(peerID string, addr string) bool {
	for _, e := range c.Entries {
		if e.PeerID == peerID && e.Addr == addr {
			return true
		}
	}
	return false
}

func (c Cache) upsertAt(peerID string, addr string, nowMs int64, update func(Entry) Entry, lim config.ClientLimits) Cache {
	pruned := c.Prune(nowMs)
	entries := make([]Entry, 0, len(pruned.Entries)+1)
	updated := false
	for _, e := range pruned.Entries {
		if e.PeerID == peerID && e.Addr == addr {
			entries = append(entries, update(e))
			updated = true
			continue
		}
		entries = append(entries, e)
	}
	if !updated {
		entries = append(entries, update(Entry{PeerID: peerID, Addr: addr, ExpiresAtMs: nowMs + int64((30*time.Minute)/time.Millisecond)}))
	}
	// Dedup: keep first occurrence per key.
	seen := make(map[string]bool, len(entries))
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		key := e.PeerID + "\n" + e.Addr
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	// Enforce size limit by dropping worst candidates.
	if lim.MaxPeers > 0 && uint64(len(out)) > lim.MaxPeers {
		sort.Slice(out, func(i, j int) bool {
			// Worst first.
			if out[i].LastOKMs != out[j].LastOKMs {
				return out[i].LastOKMs < out[j].LastOKMs
			}
			if out[i].FailCount != out[j].FailCount {
				return out[i].FailCount > out[j].FailCount
			}
			return out[i].ExpiresAtMs < out[j].ExpiresAtMs
		})
		// Defensive clamp even though config validation already caps MaxPeers.
		maxPeers := lim.MaxPeers
		if maxPeers > 4096 {
			maxPeers = 4096
		}
		maxKeep := int(maxPeers) // #nosec G115 -- maxPeers is clamped to a small value.
		out = out[len(out)-maxKeep:]
	}
	return Cache{V: 1, UpdatedAtMs: nowMs, Entries: out}
}

func cooldownMs(failCount uint64, lim config.ClientLimits) int64 {
	base := lim.CooldownBaseMs
	maxCooldown := lim.CooldownMaxMs
	if base <= 0 {
		base = 2000
	}
	if maxCooldown < base {
		maxCooldown = base
	}
	// Exponential backoff: base * 2^(failCount-1)
	ms := base
	if failCount > 1 {
		shift := failCount - 1
		if shift > 30 {
			shift = 30
		}
		ms = base * (1 << shift)
	}
	if ms > maxCooldown {
		return maxCooldown
	}
	return ms
}
