package main

import (
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/transport/cliutil"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ufa"
)

func TestResolveNodeIDFromAlias(t *testing.T) {
	nodeID := mustNodeID(t)
	book := addressbook.Book{
		V: 1,
		Entries: []addressbook.Entry{
			{
				Alias:      "node",
				TargetType: "node",
				TargetID:   nodeID,
				Source:     "self",
				Trust:      "trusted",
			},
		},
	}
	got, err := cliutil.ResolveNodeID("NoDe", book, time.Now().UTC().UnixMilli())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nodeID {
		t.Fatalf("unexpected node id: %s", got)
	}
}

func TestResolveNodeIDDirect(t *testing.T) {
	nodeID := mustNodeID(t)
	book := addressbook.Book{V: 1, Entries: []addressbook.Entry{}}
	got, err := cliutil.ResolveNodeID(nodeID, book, time.Now().UTC().UnixMilli())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nodeID {
		t.Fatalf("unexpected node id: %s", got)
	}
}

func TestResolveNodeIDTypeMismatch(t *testing.T) {
	book := addressbook.Book{
		V: 1,
		Entries: []addressbook.Entry{
			{
				Alias:      "svc",
				TargetType: "service",
				TargetID:   "svc_x",
				Source:     "self",
				Trust:      "trusted",
			},
		},
	}
	_, err := cliutil.ResolveNodeID("svc", book, time.Now().UTC().UnixMilli())
	if err != ufa.ErrUFATypeMismatch {
		t.Fatalf("expected ErrUFATypeMismatch, got %v", err)
	}
}

func TestResolveNodeIDIdentityMismatch(t *testing.T) {
	id, err := identity.NewEphemeral()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	book := addressbook.Book{V: 1, Entries: []addressbook.Entry{}}
	_, err = cliutil.ResolveNodeID(id.ID, book, time.Now().UTC().UnixMilli())
	if err != ufa.ErrUFATypeMismatch {
		t.Fatalf("expected ErrUFATypeMismatch, got %v", err)
	}
}

func mustNodeID(t *testing.T) string {
	t.Helper()
	c, err := cid.Prefix{
		Version:  1,
		Codec:    cid.DagCBOR,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum([]byte("node"))
	if err != nil {
		t.Fatalf("cid sum: %v", err)
	}
	return c.String()
}
