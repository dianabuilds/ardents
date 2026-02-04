package discoverycache

import (
	"testing"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
)

func TestCandidates_PrunesExpiredAndCooldown(t *testing.T) {
	now := int64(1000)
	c := Cache{
		V: 1,
		Entries: []Entry{
			{PeerID: "p1", Addr: "a1", ExpiresAtMs: now + 1, LastOKMs: 10},
			{PeerID: "p2", Addr: "a2", ExpiresAtMs: now - 1, LastOKMs: 20},              // expired
			{PeerID: "p3", Addr: "a3", ExpiresAtMs: now + 10, CooldownUntilMs: now + 5}, // cooldown
		},
	}
	out := c.Candidates(now)
	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(out))
	}
	if out[0].PeerID != "p1" {
		t.Fatalf("expected p1, got %s", out[0].PeerID)
	}
}

func TestUpsertCandidate_EnforcesMaxPeers(t *testing.T) {
	now := int64(1000)
	lim := config.ClientLimits{MaxPeers: 2, CooldownBaseMs: 1000, CooldownMaxMs: 10_000}
	c := Cache{V: 1, Entries: []Entry{}}
	c = c.UpsertCandidate("p1", "a1", "bootstrap", now, now+100, lim, nil)
	c = c.UpsertCandidate("p2", "a2", "bootstrap", now, now+100, lim, nil)
	c = c.UpsertCandidate("p3", "a3", "bootstrap", now, now+100, lim, nil)
	if len(c.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(c.Entries))
	}
}

func TestMarkFail_SetsCooldown(t *testing.T) {
	now := int64(1000)
	lim := config.ClientLimits{MaxPeers: 10, CooldownBaseMs: 2000, CooldownMaxMs: 60_000}
	c := Cache{V: 1, Entries: []Entry{{PeerID: "p1", Addr: "a1", ExpiresAtMs: now + 100}}}
	c = c.MarkFail("p1", "a1", now, lim)
	if c.Entries[0].CooldownUntilMs <= now {
		t.Fatalf("expected cooldown > now, got %d", c.Entries[0].CooldownUntilMs)
	}
	out := c.Candidates(now)
	if len(out) != 0 {
		t.Fatalf("expected no candidates during cooldown, got %d", len(out))
	}
}
