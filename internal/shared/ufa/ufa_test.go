package ufa

import (
	"errors"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
)

func TestParseAlias(t *testing.T) {
	res, err := Parse("MyAlias", time.Now().UTC().UnixMilli(), func(alias string, _ int64) (string, string, bool, error) {
		if alias != "myalias" {
			t.Fatalf("expected lowercase alias, got %q", alias)
		}
		return "service", "svc_abc", true, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetType != "service" || res.TargetID != "svc_abc" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseNodeID(t *testing.T) {
	c, err := cid.Prefix{
		Version:  1,
		Codec:    cid.DagCBOR,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum([]byte("node"))
	if err != nil {
		t.Fatalf("cid sum: %v", err)
	}
	res, err := Parse(c.String(), time.Now().UTC().UnixMilli(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetType != TargetNode || res.TargetID != c.String() {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseServiceID(t *testing.T) {
	id, err := identity.NewEphemeral()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	serviceID, err := ids.NewServiceID(id.ID, "web.request.v1")
	if err != nil {
		t.Fatalf("service id: %v", err)
	}
	res, err := Parse(serviceID, time.Now().UTC().UnixMilli(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetType != TargetService || res.TargetID != serviceID {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestResolveIdentityToService(t *testing.T) {
	id, err := identity.NewEphemeral()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	res, err := Resolve(id.ID, time.Now().UTC().UnixMilli(), nil, "web.request.v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetType != TargetService {
		t.Fatalf("unexpected target type: %s", res.TargetType)
	}
	if ids.ValidateServiceID(res.TargetID) != nil {
		t.Fatalf("invalid service id: %s", res.TargetID)
	}
}

func TestResolveIdentityRequiresServiceName(t *testing.T) {
	id, err := identity.NewEphemeral()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	_, err = Resolve(id.ID, time.Now().UTC().UnixMilli(), nil, "")
	if !errors.Is(err, ErrServiceNameRequired) {
		t.Fatalf("expected ErrServiceNameRequired, got %v", err)
	}
}

func TestParseRejectsWhitespace(t *testing.T) {
	_, err := Parse("  a b  ", time.Now().UTC().UnixMilli(), nil)
	if !errors.Is(err, ErrUFAUnsupported) {
		t.Fatalf("expected ErrUFAUnsupported, got %v", err)
	}
}
