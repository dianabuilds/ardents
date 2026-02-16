package app

import (
	"errors"
	"sort"
	"strings"
)

var ErrInvalidIdentityID = errors.New("invalid identity id")

func NormalizeIdentityID(identityID string) (string, error) {
	identityID = strings.TrimSpace(identityID)
	if !strings.HasPrefix(identityID, "aim1") || len(identityID) < 12 {
		return "", ErrInvalidIdentityID
	}
	return identityID, nil
}

type Blocklist struct {
	entries map[string]struct{}
}

func NewBlocklist(ids []string) (Blocklist, error) {
	b := Blocklist{entries: make(map[string]struct{}, len(ids))}
	for _, raw := range ids {
		id, err := NormalizeIdentityID(raw)
		if err != nil {
			return Blocklist{}, err
		}
		b.entries[id] = struct{}{}
	}
	return b, nil
}

func (b *Blocklist) Add(identityID string) error {
	if b.entries == nil {
		b.entries = make(map[string]struct{})
	}
	id, err := NormalizeIdentityID(identityID)
	if err != nil {
		return err
	}
	b.entries[id] = struct{}{}
	return nil
}

func (b *Blocklist) Remove(identityID string) error {
	id, err := NormalizeIdentityID(identityID)
	if err != nil {
		return err
	}
	delete(b.entries, id)
	return nil
}

func (b Blocklist) Contains(identityID string) bool {
	id, err := NormalizeIdentityID(identityID)
	if err != nil {
		return false
	}
	_, ok := b.entries[id]
	return ok
}

func (b Blocklist) List() []string {
	out := make([]string, 0, len(b.entries))
	for id := range b.entries {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
