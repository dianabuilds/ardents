package addressbook

import (
	"testing"

	"github.com/dianabuilds/ardents/internal/shared/identity"
)

func TestIsTrustedIdentity(t *testing.T) {
	b := Book{
		V: 1,
		Entries: []Entry{
			{TargetType: "identity", TargetID: "id1", Trust: "trusted"},
			{TargetType: "identity", TargetID: "id2", Trust: "untrusted"},
		},
	}
	if !b.IsTrustedIdentity("id1", 0) {
		t.Fatal("expected trusted")
	}
	if b.IsTrustedIdentity("id2", 0) {
		t.Fatal("expected untrusted")
	}
	if b.IsTrustedIdentity("missing", 0) {
		t.Fatal("expected untrusted for missing")
	}
}

func TestResolveAliasConflictRules(t *testing.T) {
	b := Book{
		V: 1,
		Entries: []Entry{
			{Alias: "aa", TargetType: "peer", TargetID: "z1", Trust: "untrusted", Source: "imported", CreatedAtMs: 1},
			{Alias: "aa", TargetType: "peer", TargetID: "z2", Trust: "trusted", Source: "imported", CreatedAtMs: 1},
			{Alias: "aa", TargetType: "peer", TargetID: "z3", Trust: "trusted", Source: "self", CreatedAtMs: 1},
			{Alias: "aa", TargetType: "peer", TargetID: "z0", Trust: "trusted", Source: "self", CreatedAtMs: 2},
		},
	}
	best, ok, err := b.ResolveAlias("aa", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected resolve")
	}
	if best.TargetID != "z0" {
		t.Fatalf("expected z0, got %s", best.TargetID)
	}
}

func TestResolveAliasExpires(t *testing.T) {
	now := int64(100)
	b := Book{
		V: 1,
		Entries: []Entry{
			{Alias: "aa", TargetType: "peer", TargetID: "z1", Trust: "trusted", Source: "self", CreatedAtMs: 1, ExpiresAtMs: 50},
			{Alias: "aa", TargetType: "peer", TargetID: "z2", Trust: "trusted", Source: "self", CreatedAtMs: 2},
		},
	}
	best, ok, err := b.ResolveAlias("aa", now)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || best.TargetID != "z2" {
		t.Fatal("expected unexpired entry")
	}
}

func TestResolveAliasConflict(t *testing.T) {
	b := Book{
		V: 1,
		Entries: []Entry{
			{Alias: "bb", TargetType: "peer", TargetID: "z1", Trust: "trusted", Source: "self", CreatedAtMs: 1},
			{Alias: "bb", TargetType: "service", TargetID: "z1", Trust: "trusted", Source: "self", CreatedAtMs: 1},
		},
	}
	_, _, err := b.ResolveAlias("bb", 0)
	if err != ErrAliasConflict {
		t.Fatalf("expected ERR_ALIAS_CONFLICT, got %v", err)
	}
}

func TestBundleExportImport(t *testing.T) {
	id, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := Book{
		V: 1,
		Entries: []Entry{
			{Alias: "aa", TargetType: "identity", TargetID: id.ID, Trust: "trusted", Source: "self", CreatedAtMs: 1},
		},
	}
	node, err := b.ExportBundle(id)
	if err != nil {
		t.Fatal(err)
	}
	// importer trusts author
	imp := Book{
		V: 1,
		Entries: []Entry{
			{Alias: "author", TargetType: "identity", TargetID: id.ID, Trust: "trusted", Source: "self", CreatedAtMs: 1},
		},
	}
	imp, err = imp.ImportBundle(node, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Entries) < 2 {
		t.Fatal("expected imported entries")
	}
}

func TestBundleImportUntrusted(t *testing.T) {
	id, err := identity.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := Book{
		V: 1,
		Entries: []Entry{
			{Alias: "aa", TargetType: "identity", TargetID: id.ID, Trust: "trusted", Source: "self", CreatedAtMs: 1},
		},
	}
	node, err := b.ExportBundle(id)
	if err != nil {
		t.Fatal(err)
	}
	imp := Book{V: 1, Entries: []Entry{}}
	_, err = imp.ImportBundle(node, 10)
	if err == nil {
		t.Fatal("expected untrusted import error")
	}
}
