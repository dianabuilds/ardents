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
	ErrDomainInvalid   = errors.New("ERR_DOMAIN_INVALID")
	ErrDomainConflict  = errors.New("ERR_DOMAIN_CONFLICT")
	ErrDomainUntrusted = errors.New("ERR_DOMAIN_UNTRUSTED")
)

type Book struct {
	V             uint64   `json:"v"`
	UpdatedAtMs   int64    `json:"updated_at_ms"`
	Entries       []Entry  `json:"entries"`
	RevokedIDs    []string `json:"revoked_identity_ids,omitempty"`
	DeprecatedIDs []string `json:"deprecated_identity_ids,omitempty"`

	idx *bookIndex
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

type ImportStats struct {
	Total   int
	Added   int
	Skipped int
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
	b.idx = buildIndex(b)
	if err := Save(path, b); err != nil {
		return Book{}, err
	}
	return b, nil
}

func Load(path string) (Book, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is controlled by app dirs.
	if err != nil {
		return Book{}, err
	}
	var b Book
	if err := json.Unmarshal(data, &b); err != nil {
		return Book{}, err
	}
	b.idx = buildIndex(b)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (b Book) IsTrustedIdentity(identityID string, nowMs int64) bool {
	idx := b.index()
	for _, i := range idx.trustedIdentity[identityID] {
		e := b.Entries[i]
		if e.ExpiresAtMs != 0 && nowMs > e.ExpiresAtMs {
			continue
		}
		return true
	}
	return false
}

func (b Book) IsRevokedIdentity(identityID string) bool {
	idx := b.index()
	return idx.revoked[identityID]
}

func (b Book) IsDeprecatedIdentity(identityID string) bool {
	idx := b.index()
	return idx.deprecated[identityID]
}

func (b Book) TrustedPeers(nowMs int64) map[string]bool {
	out := make(map[string]bool)
	idx := b.index()
	for peerID, entries := range idx.trustedPeers {
		for _, i := range entries {
			e := b.Entries[i]
			if e.ExpiresAtMs != 0 && nowMs > e.ExpiresAtMs {
				continue
			}
			out[peerID] = true
			break
		}
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

func (b Book) ImportBundle(node contentnode.Node, nowMs int64) (Book, ImportStats, error) {
	if node.Type != "bundle.addressbook.v1" {
		return b, ImportStats{}, ErrBundleInvalid
	}
	if err := contentnode.Verify(&node); err != nil {
		return b, ImportStats{}, ErrBundleInvalid
	}
	if !b.IsTrustedIdentity(node.Owner, nowMs) {
		return b, ImportStats{}, ErrImportUntrusted
	}
	body := normalizeMap(node.Body)
	if err := importIDList(body["revoked_identity_ids"], &b.RevokedIDs); err != nil {
		return b, ImportStats{}, ErrBundleInvalid
	}
	if err := importIDList(body["deprecated_identity_ids"], &b.DeprecatedIDs); err != nil {
		return b, ImportStats{}, ErrBundleInvalid
	}
	entries, total, ok := importEntries(body["entries"], nowMs)
	if !ok {
		return b, ImportStats{}, ErrBundleInvalid
	}
	b.Entries = append(b.Entries, entries...)
	b.UpdatedAtMs = nowMs
	b.idx = buildIndex(b)
	stats := ImportStats{
		Total:   total,
		Added:   len(entries),
		Skipped: total - len(entries),
	}
	return b, stats, nil
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

func importEntries(raw any, nowMs int64) ([]Entry, int, bool) {
	switch v := raw.(type) {
	case []Entry:
		entries := normalizeEntries(v, nowMs)
		return entries, len(v), true
	case []any:
		entries := buildEntriesFromAny(v, nowMs)
		return entries, len(v), true
	default:
		return nil, 0, false
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
		if e.ExpiresAtMs == 0 || e.ExpiresAtMs <= nowMs {
			continue
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
		if exp, ok := asInt64(obj["expires_at_ms"]); ok {
			entry.ExpiresAtMs = exp
		}
		if entry.ExpiresAtMs == 0 || entry.ExpiresAtMs <= nowMs {
			continue
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

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case json.Number:
		if v, err := n.Int64(); err == nil {
			return v, true
		}
	}
	return 0, false
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
	idx := b.index()
	ids := idx.aliasToEntries[alias]
	candidates := make([]Entry, 0, len(ids))
	for _, i := range ids {
		e := b.Entries[i]
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

func (b Book) ResolveDomain(alias string, nowMs int64) (Entry, bool, error) {
	entry, ok, err := b.ResolveAlias(alias, nowMs)
	if err != nil {
		if errors.Is(err, ErrAliasInvalid) {
			return Entry{}, false, ErrDomainInvalid
		}
		if errors.Is(err, ErrAliasConflict) {
			return Entry{}, false, ErrDomainConflict
		}
		return Entry{}, false, err
	}
	if !ok {
		return Entry{}, false, nil
	}
	if entry.Trust != "trusted" {
		return Entry{}, false, ErrDomainUntrusted
	}
	return entry, true, nil
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

// RebuildIndex recomputes in-memory indices for hot-path operations (resolve, trust checks).
// It doesn't affect the serialized JSON format.
func (b *Book) RebuildIndex() {
	if b == nil {
		return
	}
	b.idx = buildIndex(*b)
}

type bookIndex struct {
	updatedAtMs   int64
	entriesLen    int
	revokedLen    int
	deprecatedLen int

	aliasToEntries  map[string][]int
	trustedIdentity map[string][]int
	trustedPeers    map[string][]int
	revoked         map[string]bool
	deprecated      map[string]bool
}

func (b Book) index() *bookIndex {
	if b.idx != nil &&
		b.idx.updatedAtMs == b.UpdatedAtMs &&
		b.idx.entriesLen == len(b.Entries) &&
		b.idx.revokedLen == len(b.RevokedIDs) &&
		b.idx.deprecatedLen == len(b.DeprecatedIDs) {
		return b.idx
	}
	return buildIndex(b)
}

func buildIndex(b Book) *bookIndex {
	idx := &bookIndex{
		updatedAtMs:     b.UpdatedAtMs,
		entriesLen:      len(b.Entries),
		revokedLen:      len(b.RevokedIDs),
		deprecatedLen:   len(b.DeprecatedIDs),
		aliasToEntries:  make(map[string][]int),
		trustedIdentity: make(map[string][]int),
		trustedPeers:    make(map[string][]int),
		revoked:         make(map[string]bool),
		deprecated:      make(map[string]bool),
	}

	for i, e := range b.Entries {
		if e.Alias != "" {
			idx.aliasToEntries[e.Alias] = append(idx.aliasToEntries[e.Alias], i)
		}
		if e.Trust != "trusted" {
			continue
		}
		switch e.TargetType {
		case "identity":
			if e.TargetID != "" {
				idx.trustedIdentity[e.TargetID] = append(idx.trustedIdentity[e.TargetID], i)
			}
		case "peer":
			if e.TargetID != "" {
				idx.trustedPeers[e.TargetID] = append(idx.trustedPeers[e.TargetID], i)
			}
		}
	}
	for _, id := range b.RevokedIDs {
		if id == "" {
			continue
		}
		idx.revoked[id] = true
	}
	for _, id := range b.DeprecatedIDs {
		if id == "" {
			continue
		}
		idx.deprecated[id] = true
	}
	return idx
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
