package privacy

import (
	"errors"
	"testing"
)

func TestBlocklistAddRemoveContainsList(t *testing.T) {
	list, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	idA := "aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF"
	idB := "aim1rpcpartialcontact02"

	if err := list.Add(idA); err != nil {
		t.Fatalf("add A failed: %v", err)
	}
	if err := list.Add(idB); err != nil {
		t.Fatalf("add B failed: %v", err)
	}
	if !list.Contains(idA) || !list.Contains(idB) {
		t.Fatal("contains should return true for added ids")
	}

	got := list.List()
	if len(got) != 2 {
		t.Fatalf("unexpected list size: got=%d want=2", len(got))
	}
	if got[0] > got[1] {
		t.Fatal("list must return deterministic sorted order")
	}

	if err := list.Remove(idA); err != nil {
		t.Fatalf("remove A failed: %v", err)
	}
	if list.Contains(idA) {
		t.Fatal("removed id must not be present")
	}
}

func TestBlocklistRejectsInvalidIdentityID(t *testing.T) {
	list, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	if err := list.Add("not-aim-id"); !errors.Is(err, ErrInvalidIdentityID) {
		t.Fatalf("expected ErrInvalidIdentityID, got %v", err)
	}
	if err := list.Remove("bad-id"); !errors.Is(err, ErrInvalidIdentityID) {
		t.Fatalf("expected ErrInvalidIdentityID on remove, got %v", err)
	}
}

func TestNewBlocklistRejectsInvalidInput(t *testing.T) {
	_, err := NewBlocklist([]string{"aim1okid12345", "invalid"})
	if !errors.Is(err, ErrInvalidIdentityID) {
		t.Fatalf("expected ErrInvalidIdentityID, got %v", err)
	}
}
