package main

import (
	"errors"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/transport/cliutil"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/ufa"
)

func TestResolveTargetAliasService(t *testing.T) {
	id, err := identity.NewEphemeral()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	serviceID, err := ids.NewServiceID(id.ID, "web.request.v1")
	if err != nil {
		t.Fatalf("service id: %v", err)
	}
	book := addressbook.Book{
		V: 1,
		Entries: []addressbook.Entry{
			{
				Alias:       "web",
				TargetType:  "service",
				TargetID:    serviceID,
				Source:      "self",
				Trust:       "trusted",
				CreatedAtMs: time.Now().UTC().UnixMilli(),
			},
		},
	}
	got, _, err := cliutil.ResolveServiceID("web", "web.request.v1", book, time.Now().UTC().UnixMilli())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != serviceID {
		t.Fatalf("unexpected target: %s", got)
	}
}

func TestResolveTargetIdentity(t *testing.T) {
	id, err := identity.NewEphemeral()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	book := addressbook.Book{V: 1, Entries: []addressbook.Entry{}}
	got, gotIdentity, err := cliutil.ResolveServiceID(id.ID, "web.request.v1", book, time.Now().UTC().UnixMilli())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids.ValidateServiceID(got) != nil {
		t.Fatalf("invalid service id: %s", got)
	}
	if gotIdentity == "" {
		t.Fatal("expected target identity id")
	}
}

func TestResolveTargetServiceID(t *testing.T) {
	id, err := identity.NewEphemeral()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	serviceID, err := ids.NewServiceID(id.ID, "web.request.v1")
	if err != nil {
		t.Fatalf("service id: %v", err)
	}
	book := addressbook.Book{V: 1, Entries: []addressbook.Entry{}}
	got, _, err := cliutil.ResolveServiceID(serviceID, "web.request.v1", book, time.Now().UTC().UnixMilli())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != serviceID {
		t.Fatalf("unexpected service id: %s", got)
	}
}

func TestResolveTargetTypeMismatch(t *testing.T) {
	book := addressbook.Book{
		V: 1,
		Entries: []addressbook.Entry{
			{
				Alias:       "node",
				TargetType:  "node",
				TargetID:    "cidv1-placeholder",
				Source:      "self",
				Trust:       "trusted",
				CreatedAtMs: time.Now().UTC().UnixMilli(),
			},
		},
	}
	_, _, err := cliutil.ResolveServiceID("node", "web.request.v1", book, time.Now().UTC().UnixMilli())
	if !errors.Is(err, ufa.ErrUFATypeMismatch) {
		t.Fatalf("expected ErrUFATypeMismatch, got %v", err)
	}
}
