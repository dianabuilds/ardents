package cliutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/ufa"
)

func TestResolveNodeID_Alias(t *testing.T) {
	nowMs := time.Now().UTC().UnixMilli()
	_, cidStr, err := contentnode.EncodeWithCID(contentnode.Node{
		V:           1,
		Type:        "test.node.v1",
		CreatedAtMs: nowMs,
		Owner:       "did:key:z6Mktest", // not verified by UFA parsing
		Links:       []contentnode.Link{},
		Body:        map[string]any{"k": "v"},
		Policy:      map[string]any{"v": uint64(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	book := addressbook.Book{
		V:           1,
		UpdatedAtMs: nowMs,
		Entries: []addressbook.Entry{
			{
				Alias:       "nn",
				TargetType:  "node",
				TargetID:    cidStr,
				Source:      "local",
				Trust:       "trusted",
				CreatedAtMs: nowMs,
				ExpiresAtMs: nowMs + 60_000,
			},
		},
	}
	got, err := ResolveNodeID("nn", book, nowMs)
	if err != nil {
		t.Fatal(err)
	}
	if got != cidStr {
		t.Fatalf("unexpected node id: got=%q want=%q", got, cidStr)
	}
}

func TestResolveNodeID_TypeMismatch(t *testing.T) {
	nowMs := time.Now().UTC().UnixMilli()
	book := addressbook.Book{V: 1, UpdatedAtMs: nowMs, Entries: []addressbook.Entry{}}
	_, err := ResolveNodeID("did:key:z6MkrejUodV7pAM4gA1e2B2PfNBzm4Pmje6A6xbFAdotGyzK", book, nowMs)
	if !errors.Is(err, ufa.ErrUFATypeMismatch) {
		t.Fatalf("expected type mismatch, got: %v", err)
	}
}

func TestResolveServiceID_FromIdentity(t *testing.T) {
	nowMs := time.Now().UTC().UnixMilli()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identityID, err := ids.NewIdentityID(pub)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ids.NewServiceID(identityID, "web.request.v1")
	if err != nil {
		t.Fatal(err)
	}
	book := addressbook.Book{V: 1, UpdatedAtMs: nowMs, Entries: []addressbook.Entry{}}
	got, targetID, err := ResolveServiceID(identityID, "web.request.v1", book, nowMs)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("unexpected service id: got=%q want=%q", got, want)
	}
	if targetID != identityID {
		t.Fatalf("unexpected target identity id: got=%q want=%q", targetID, identityID)
	}
}

func TestResolveServiceID_AliasToServiceID(t *testing.T) {
	nowMs := time.Now().UTC().UnixMilli()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identityID, err := ids.NewIdentityID(pub)
	if err != nil {
		t.Fatal(err)
	}
	serviceID, err := ids.NewServiceID(identityID, "web.request.v1")
	if err != nil {
		t.Fatal(err)
	}
	book := addressbook.Book{
		V:           1,
		UpdatedAtMs: nowMs,
		Entries: []addressbook.Entry{
			{
				Alias:       "web",
				TargetType:  "service",
				TargetID:    serviceID,
				Source:      "local",
				Trust:       "trusted",
				CreatedAtMs: nowMs,
				ExpiresAtMs: nowMs + 60_000,
			},
		},
	}
	got, targetID, err := ResolveServiceID("web", "web.request.v1", book, nowMs)
	if err != nil {
		t.Fatal(err)
	}
	if got != serviceID {
		t.Fatalf("unexpected service id: got=%q want=%q", got, serviceID)
	}
	if targetID != "" {
		t.Fatalf("expected empty target identity id for alias, got=%q", targetID)
	}
}
