package addressbook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/identity"
)

const DefaultPath = "data/addressbook.json"

var (
	ErrBundleInvalid   = errors.New("ERR_ADDRESSBOOK_BUNDLE_INVALID")
	ErrImportUntrusted = errors.New("ERR_IMPORT_UNTRUSTED_SOURCE")
	ErrEntryInvalid    = errors.New("ERR_ADDRESSBOOK_ENTRY_INVALID")
	ErrAliasInvalid    = errors.New("ERR_ALIAS_INVALID")
	ErrAliasConflict   = errors.New("ERR_ALIAS_CONFLICT")
)

type Book struct {
	V             uint64   `json:"v"`
	UpdatedAtMs   int64    `json:"updated_at_ms"`
	Entries       []Entry  `json:"entries"`
	RevokedIDs    []string `json:"revoked_identity_ids,omitempty"`
	DeprecatedIDs []string `json:"deprecated_identity_ids,omitempty"`
}

type Entry struct {
	Alias       string `json:"alias"`
	TargetType  string `json:"target_type"`
	TargetID    string `json:"target_id"`
	Source      string `json:"source"`
	Trust       string `json:"trust"`
	Note        string `json:"note,omitempty"`
	CreatedAtMs int64  `json:"created_at_ms"`
	ExpiresAtMs int64  `json:"expires_at_ms,omitempty"`
}

func LoadOrInit(path string) (Book, error) {
	if path == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			path = d.AddressBookPath()
		} else {
			path = DefaultPath
		}
	}
	if _, err := os.Stat(path); err == nil {
		return Load(path)
	}
	b := Book{
		V:           1,
		UpdatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Entries:     []Entry{},
	}
	if err := Save(path, b); err != nil {
		return Book{}, err
	}
	return b, nil
}

func Load(path string) (Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Book{}, err
	}
	var b Book
	if err := json.Unmarshal(data, &b); err != nil {
		return Book{}, err
	}
	return b, nil
}

func Save(path string, b Book) error {
	if path == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			path = d.AddressBookPath()
		} else {
			path = DefaultPath
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (b Book) IsTrustedIdentity(identityID string, nowMs int64) bool {
	for _, e := range b.Entries {
		if e.TargetType != "identity" || e.Trust != "trusted" {
			continue
		}
		if e.TargetID != identityID {
			continue
		}
		if e.ExpiresAtMs != 0 && nowMs > e.ExpiresAtMs {
			continue
		}
		return true
	}
	return false
}

func (b Book) IsRevokedIdentity(identityID string) bool {
	for _, id := range b.RevokedIDs {
		if id == identityID {
			return true
		}
	}
	return false
}

func (b Book) IsDeprecatedIdentity(identityID string) bool {
	for _, id := range b.DeprecatedIDs {
		if id == identityID {
			return true
		}
	}
	return false
}

func (b Book) TrustedPeers(nowMs int64) map[string]bool {
	out := make(map[string]bool)
	for _, e := range b.Entries {
		if e.TargetType != "peer" || e.Trust != "trusted" {
			continue
		}
		if e.ExpiresAtMs != 0 && nowMs > e.ExpiresAtMs {
			continue
		}
		out[e.TargetID] = true
	}
	return out
}

func (b Book) ExportBundle(author identity.Identity) (contentnode.Node, error) {
	body := map[string]any{
		"entries":                 b.Entries,
		"revoked_identity_ids":    b.RevokedIDs,
		"deprecated_identity_ids": b.DeprecatedIDs,
	}
	n := contentnode.Node{
		V:           1,
		Type:        "bundle.addressbook.v1",
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Owner:       author.ID,
		Links:       []contentnode.Link{},
		Body:        body,
		Policy: map[string]any{
			"v":          uint64(1),
			"visibility": "public",
		},
	}
	if err := contentnode.Sign(&n, author.PrivateKey); err != nil {
		return contentnode.Node{}, err
	}
	return n, nil
}

func (b Book) ImportBundle(node contentnode.Node, nowMs int64) (Book, error) {
	if node.Type != "bundle.addressbook.v1" {
		return b, ErrBundleInvalid
	}
	if err := contentnode.Verify(&node); err != nil {
		return b, ErrBundleInvalid
	}
	if !b.IsTrustedIdentity(node.Owner, nowMs) {
		return b, ErrImportUntrusted
	}
	body := normalizeMap(node.Body)
	if err := importIDList(body["revoked_identity_ids"], &b.RevokedIDs); err != nil {
		return b, ErrBundleInvalid
	}
	if err := importIDList(body["deprecated_identity_ids"], &b.DeprecatedIDs); err != nil {
		return b, ErrBundleInvalid
	}
	entries, ok := importEntries(body["entries"], nowMs)
	if !ok {
		return b, ErrBundleInvalid
	}
	b.Entries = append(b.Entries, entries...)
	b.UpdatedAtMs = nowMs
	return b, nil
}

func importIDList(raw any, dst *[]string) error {
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			id := asString(item)
			if id == "" {
				continue
			}
			*dst = appendUnique(*dst, id)
		}
	case []string:
		for _, id := range v {
			*dst = appendUnique(*dst, id)
		}
	case nil:
		return nil
	default:
		return ErrBundleInvalid
	}
	return nil
}

func importEntries(raw any, nowMs int64) ([]Entry, bool) {
	switch v := raw.(type) {
	case []Entry:
		return normalizeEntries(v, nowMs), true
	case []any:
		return buildEntriesFromAny(v, nowMs), true
	default:
		return nil, false
	}
}

func normalizeEntries(entries []Entry, nowMs int64) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		e.Source = "imported"
		if e.Trust == "" {
			e.Trust = "untrusted"
		}
		if e.CreatedAtMs == 0 {
			e.CreatedAtMs = nowMs
		}
		if aliasErr := validateEntry(e); aliasErr != nil {
			continue
		}
		out = append(out, e)
	}
	return out
}

func buildEntriesFromAny(rawEntries []any, nowMs int64) []Entry {
	out := make([]Entry, 0, len(rawEntries))
	for _, re := range rawEntries {
		obj := normalizeMap(re)
		entry := Entry{
			Alias:       asString(obj["alias"]),
			TargetType:  asString(obj["target_type"]),
			TargetID:    asString(obj["target_id"]),
			Source:      "imported",
			Trust:       asStringDefault(obj["trust"], "untrusted"),
			CreatedAtMs: nowMs,
		}
		if exp, ok := obj["expires_at_ms"].(int64); ok {
			entry.ExpiresAtMs = exp
		}
		if aliasErr := validateEntry(entry); aliasErr != nil {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func appendUnique(list []string, id string) []string {
	for _, v := range list {
		if v == id {
			return list
		}
	}
	return append(list, id)
}

func validateEntry(e Entry) error {
	if e.Alias == "" || e.TargetID == "" {
		return ErrEntryInvalid
	}
	if err := validateAlias(e.Alias); err != nil {
		return err
	}
	return nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asStringDefault(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func normalizeMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = val
		}
		return out
	default:
		return map[string]any{}
	}
}

var aliasPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}[a-z0-9]$`)

func validateAlias(alias string) error {
	if !aliasPattern.MatchString(alias) {
		return ErrAliasInvalid
	}
	return nil
}

func (b Book) ResolveAlias(alias string, nowMs int64) (Entry, bool, error) {
	if err := validateAlias(alias); err != nil {
		return Entry{}, false, err
	}
	candidates := make([]Entry, 0)
	for _, e := range b.Entries {
		if e.Alias != alias {
			continue
		}
		if e.ExpiresAtMs != 0 && nowMs > e.ExpiresAtMs {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		return Entry{}, false, nil
	}
	best := candidates[0]
	for i := 1; i < len(candidates); i++ {
		best = pickBetter(best, candidates[i])
	}
	for _, cand := range candidates {
		if cand.TargetID != best.TargetID {
			continue
		}
		if sameRank(best, cand) && cand.TargetType != best.TargetType {
			return Entry{}, false, ErrAliasConflict
		}
	}
	return best, true, nil
}

func sameRank(a, b Entry) bool {
	if a.Trust != b.Trust {
		return false
	}
	if a.Source != b.Source {
		return false
	}
	if a.CreatedAtMs != b.CreatedAtMs {
		return false
	}
	return true
}

func pickBetter(a, b Entry) Entry {
	trustRank := func(t string) int {
		if t == "trusted" {
			return 2
		}
		return 1
	}
	sourceRank := func(s string) int {
		if s == "self" {
			return 2
		}
		return 1
	}
	if trustRank(b.Trust) > trustRank(a.Trust) {
		return b
	}
	if trustRank(b.Trust) < trustRank(a.Trust) {
		return a
	}
	if sourceRank(b.Source) > sourceRank(a.Source) {
		return b
	}
	if sourceRank(b.Source) < sourceRank(a.Source) {
		return a
	}
	if b.CreatedAtMs > a.CreatedAtMs {
		return b
	}
	if b.CreatedAtMs < a.CreatedAtMs {
		return a
	}
	if b.TargetID < a.TargetID {
		return b
	}
	return a
}
