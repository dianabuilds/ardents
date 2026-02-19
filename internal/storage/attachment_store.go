package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/pkg/models"
)

var ErrAttachmentNotFound = errors.New("attachment not found")
var ErrAttachmentHardCapReached = errors.New("attachment class hard cap reached")
var ErrUnsupportedStorageSchema = errors.New("unsupported storage schema version")

const attachmentIndexSchemaVersion = 2

type AttachmentStore struct {
	mu        sync.RWMutex
	dir       string
	indexPath string
	secret    string
	items     map[string]models.AttachmentMeta
	blobs     map[string][]byte
	persist   bool
	limits    attachmentClassLimits
	hardCap   attachmentHardCapPolicy
	hits      attachmentHardCapHits
}

type attachmentClassLimits struct {
	ImageQuotaBytes   int64
	ImageMaxItemBytes int64
	FileQuotaBytes    int64
	FileMaxItemBytes  int64
}

type attachmentHardCapPolicy struct {
	HighWatermarkPercent    int
	FullCapPercent          int
	AggressiveTargetPercent int
}

type attachmentHardCapHits struct {
	ImageFullCapHits int
	FileFullCapHits  int
}

type AttachmentGCReport struct {
	DryRun         bool           `json:"dry_run"`
	DeletedCount   int            `json:"deleted_count"`
	DeletedByClass map[string]int `json:"deleted_by_class"`
	DeletedByCause map[string]int `json:"deleted_by_cause"`
}

func NewAttachmentStore(dir string) (*AttachmentStore, error) {
	return NewAttachmentStoreWithSecret(dir, "")
}

func NewAttachmentStoreWithSecret(dir, secret string) (*AttachmentStore, error) {
	s := &AttachmentStore{
		dir:     dir,
		secret:  strings.TrimSpace(secret),
		items:   make(map[string]models.AttachmentMeta),
		blobs:   make(map[string][]byte),
		persist: true,
		hardCap: attachmentHardCapPolicy{
			HighWatermarkPercent:    90,
			FullCapPercent:          100,
			AggressiveTargetPercent: 75,
		},
	}
	if dir != "" {
		s.indexPath = filepath.Join(dir, "index.json")
		if err := s.load(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *AttachmentStore) Put(name, mimeType string, data []byte) (models.AttachmentMeta, error) {
	if len(data) == 0 {
		return models.AttachmentMeta{}, errors.New("attachment data is empty")
	}
	id, err := newAttachmentID()
	if err != nil {
		return models.AttachmentMeta{}, err
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	now := time.Now().UTC()
	meta := models.AttachmentMeta{
		ID:           id,
		Name:         name,
		MimeType:     mimeType,
		Class:        string(models.ClassifyAttachmentMime(mimeType)),
		LastAccessAt: now,
		PinState:     string(models.AttachmentPinStateUnpinned),
		Size:         int64(len(data)),
		CreatedAt:    now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.enforceClassLimitsLocked(meta.Class, meta.Size); err != nil {
		return models.AttachmentMeta{}, err
	}
	if s.dir != "" {
		if !s.persist {
			s.items[id] = meta
			s.blobs[id] = append([]byte(nil), data...)
			return meta, nil
		}
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return models.AttachmentMeta{}, err
		}
		blob := append([]byte(nil), data...)
		if s.secret != "" {
			blob, err = securestore.Encrypt(s.secret, blob)
			if err != nil {
				return models.AttachmentMeta{}, err
			}
		}
		filePath := s.filePath(id)
		if err := os.WriteFile(filePath, blob, 0o600); err != nil {
			return models.AttachmentMeta{}, err
		}
		nextItems := cloneAttachmentMetaMap(s.items)
		nextItems[id] = meta
		if err := s.persistItemsLocked(nextItems); err != nil {
			_ = os.Remove(filePath)
			return models.AttachmentMeta{}, err
		}
		s.items = nextItems
		return meta, nil
	}
	s.items[id] = meta
	return meta, nil
}

func (s *AttachmentStore) Get(id string) (models.AttachmentMeta, []byte, error) {
	s.mu.RLock()
	meta, ok := s.items[id]
	blob, hasBlob := s.blobs[id]
	s.mu.RUnlock()
	if !ok {
		return models.AttachmentMeta{}, nil, ErrAttachmentNotFound
	}
	if hasBlob {
		_ = s.touchLastAccess(id, time.Now().UTC())
		return meta, append([]byte(nil), blob...), nil
	}
	if s.dir == "" {
		return meta, nil, ErrAttachmentNotFound
	}
	data, err := os.ReadFile(s.filePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return models.AttachmentMeta{}, nil, ErrAttachmentNotFound
		}
		return models.AttachmentMeta{}, nil, err
	}
	if s.secret == "" {
		_ = s.touchLastAccess(id, time.Now().UTC())
		return meta, data, nil
	}
	plain, err := securestore.Decrypt(s.secret, data)
	if err != nil {
		// Backward compatibility for pre-protected plaintext attachments.
		if errors.Is(err, securestore.ErrLegacyData) {
			_ = s.touchLastAccess(id, time.Now().UTC())
			return meta, data, nil
		}
		return models.AttachmentMeta{}, nil, err
	}
	_ = s.touchLastAccess(id, time.Now().UTC())
	return meta, plain, nil
}

func (s *AttachmentStore) Wipe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]models.AttachmentMeta)
	s.blobs = make(map[string][]byte)
	if strings.TrimSpace(s.dir) == "" {
		return nil
	}
	if err := os.RemoveAll(s.dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *AttachmentStore) SetPersistenceEnabled(enabled bool) {
	s.mu.Lock()
	s.persist = enabled
	s.mu.Unlock()
}

func (s *AttachmentStore) PurgeOlderThan(cutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deletedIDs := make([]string, 0)
	for id, meta := range s.items {
		if !meta.CreatedAt.After(cutoff) {
			deletedIDs = append(deletedIDs, id)
		}
	}
	if len(deletedIDs) == 0 {
		return 0, nil
	}
	if err := s.deleteIDsLocked(deletedIDs); err != nil {
		return 0, err
	}
	return len(deletedIDs), nil
}

func (s *AttachmentStore) PurgeOlderThanByClass(class string, cutoff time.Time) (int, error) {
	class = strings.ToLower(strings.TrimSpace(class))
	if class == "" {
		return 0, errors.New("attachment class is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	deletedIDs := make([]string, 0)
	for id, meta := range s.items {
		metaClass := strings.ToLower(strings.TrimSpace(meta.Class))
		if metaClass == "" {
			metaClass = string(models.ClassifyAttachmentMime(meta.MimeType))
		}
		if metaClass == class && !meta.CreatedAt.After(cutoff) {
			deletedIDs = append(deletedIDs, id)
		}
	}
	if len(deletedIDs) == 0 {
		return 0, nil
	}
	if err := s.deleteIDsLocked(deletedIDs); err != nil {
		return 0, err
	}
	return len(deletedIDs), nil
}

func (s *AttachmentStore) SetClassPolicies(imageQuotaMB, imageMaxItemSizeMB, fileQuotaMB, fileMaxItemSizeMB int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = attachmentClassLimits{
		ImageQuotaBytes:   mbToBytes(imageQuotaMB),
		ImageMaxItemBytes: mbToBytes(imageMaxItemSizeMB),
		FileQuotaBytes:    mbToBytes(fileQuotaMB),
		FileMaxItemBytes:  mbToBytes(fileMaxItemSizeMB),
	}
}

func (s *AttachmentStore) SetHardCapPolicy(highWatermarkPercent, fullCapPercent, aggressiveTargetPercent int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if highWatermarkPercent < 1 {
		highWatermarkPercent = 1
	}
	if highWatermarkPercent > 100 {
		highWatermarkPercent = 100
	}
	if fullCapPercent < highWatermarkPercent {
		fullCapPercent = highWatermarkPercent
	}
	if fullCapPercent > 100 {
		fullCapPercent = 100
	}
	if aggressiveTargetPercent < 1 {
		aggressiveTargetPercent = 1
	}
	if aggressiveTargetPercent > highWatermarkPercent {
		aggressiveTargetPercent = highWatermarkPercent
	}
	s.hardCap = attachmentHardCapPolicy{
		HighWatermarkPercent:    highWatermarkPercent,
		FullCapPercent:          fullCapPercent,
		AggressiveTargetPercent: aggressiveTargetPercent,
	}
}

func (s *AttachmentStore) HardCapStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := map[string]int{
		"image_high_watermark_percent": s.hardCap.HighWatermarkPercent,
		"image_full_cap_percent":       s.hardCap.FullCapPercent,
		"file_high_watermark_percent":  s.hardCap.HighWatermarkPercent,
		"file_full_cap_percent":        s.hardCap.FullCapPercent,
		"image_full_cap_hits":          s.hits.ImageFullCapHits,
		"file_full_cap_hits":           s.hits.FileFullCapHits,
	}
	_, imageQuota := s.classLimitsLocked("image")
	_, fileQuota := s.classLimitsLocked("file")
	if imageQuota > 0 {
		stats["image_usage_percent"] = int((s.usageByClassLocked("image") * 100) / imageQuota)
	}
	if fileQuota > 0 {
		stats["file_usage_percent"] = int((s.usageByClassLocked("file") * 100) / fileQuota)
	}
	return stats
}

func (s *AttachmentStore) UsageByClass() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]int64{
		"image": s.usageByClassLocked("image"),
		"file":  s.usageByClassLocked("file"),
	}
}

func (s *AttachmentStore) ListMetas() []models.AttachmentMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.AttachmentMeta, 0, len(s.items))
	for _, meta := range s.items {
		out = append(out, meta)
	}
	return out
}

func (s *AttachmentStore) PutExisting(meta models.AttachmentMeta, data []byte) error {
	if strings.TrimSpace(meta.ID) == "" {
		return errors.New("attachment id is required")
	}
	if len(data) == 0 {
		return errors.New("attachment data is empty")
	}
	if strings.TrimSpace(meta.MimeType) == "" {
		meta.MimeType = "application/octet-stream"
	}
	if strings.TrimSpace(meta.Class) == "" {
		meta.Class = string(models.ClassifyAttachmentMime(meta.MimeType))
	}
	if meta.Size <= 0 {
		meta.Size = int64(len(data))
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now().UTC()
	}
	if meta.LastAccessAt.IsZero() {
		meta.LastAccessAt = meta.CreatedAt
	}
	if strings.TrimSpace(meta.PinState) == "" {
		meta.PinState = string(models.AttachmentPinStateUnpinned)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.enforceClassLimitsLocked(meta.Class, meta.Size); err != nil {
		return err
	}
	if s.dir != "" {
		if !s.persist {
			s.items[meta.ID] = meta
			s.blobs[meta.ID] = append([]byte(nil), data...)
			return nil
		}
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return err
		}
		blob := append([]byte(nil), data...)
		var err error
		if s.secret != "" {
			blob, err = securestore.Encrypt(s.secret, blob)
			if err != nil {
				return err
			}
		}
		if err := os.WriteFile(s.filePath(meta.ID), blob, 0o600); err != nil {
			return err
		}
		nextItems := cloneAttachmentMetaMap(s.items)
		nextItems[meta.ID] = meta
		if err := s.persistItemsLocked(nextItems); err != nil {
			return err
		}
		s.items = nextItems
		return nil
	}
	s.items[meta.ID] = meta
	return nil
}

func (s *AttachmentStore) SetPinState(id, pinState string) error {
	id = strings.TrimSpace(id)
	pinState = strings.ToLower(strings.TrimSpace(pinState))
	if id == "" {
		return errors.New("attachment id is required")
	}
	switch pinState {
	case string(models.AttachmentPinStatePinned), string(models.AttachmentPinStateUnpinned):
	default:
		return errors.New("invalid pin state")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, ok := s.items[id]
	if !ok {
		return ErrAttachmentNotFound
	}
	if meta.PinState == pinState {
		return nil
	}
	meta.PinState = pinState
	meta.LastAccessAt = time.Now().UTC()
	nextItems := cloneAttachmentMetaMap(s.items)
	nextItems[id] = meta
	if err := s.persistItemsLocked(nextItems); err != nil {
		return err
	}
	s.items = nextItems
	return nil
}

func (s *AttachmentStore) RunGC(now time.Time, imageTTLSeconds, fileTTLSeconds int, dryRun bool) (AttachmentGCReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runGCLocked(now, imageTTLSeconds, fileTTLSeconds, dryRun)
}

func (s *AttachmentStore) filePath(id string) string {
	return filepath.Join(s.dir, id+".bin")
}

func (s *AttachmentStore) load() error {
	if s.indexPath == "" {
		return nil
	}
	data, err := os.ReadFile(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	indexWasLegacy := false
	if s.secret != "" {
		plain, derr := securestore.Decrypt(s.secret, data)
		if derr == nil {
			data = plain
		} else if errors.Is(derr, securestore.ErrLegacyData) {
			indexWasLegacy = true
		} else {
			return derr
		}
	}
	var payload struct {
		SchemaVersion int                              `json:"schema_version"`
		Items         map[string]models.AttachmentMeta `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	schemaMigrated := false
	switch {
	case payload.SchemaVersion == 0:
		// Legacy snapshots had no explicit versioning.
		schemaMigrated = true
	case payload.SchemaVersion > attachmentIndexSchemaVersion:
		return fmt.Errorf("%w: attachments=%d current=%d", ErrUnsupportedStorageSchema, payload.SchemaVersion, attachmentIndexSchemaVersion)
	case payload.SchemaVersion < attachmentIndexSchemaVersion:
		// Current schema is backward-compatible with v1 payload shape.
		schemaMigrated = true
	}
	if payload.Items != nil {
		s.items = payload.Items
	}
	normalized := false
	for id, meta := range s.items {
		if strings.TrimSpace(meta.Class) == "" {
			meta.Class = string(models.ClassifyAttachmentMime(meta.MimeType))
			normalized = true
		}
		if strings.TrimSpace(meta.PinState) == "" {
			meta.PinState = string(models.AttachmentPinStateUnpinned)
			normalized = true
		}
		if meta.LastAccessAt.IsZero() {
			if !meta.CreatedAt.IsZero() {
				meta.LastAccessAt = meta.CreatedAt
			} else {
				meta.LastAccessAt = time.Now().UTC()
			}
			normalized = true
		}
		s.items[id] = meta
	}
	if s.secret != "" {
		if err := s.migrateLegacyFiles(payload.Items); err != nil {
			return err
		}
		if indexWasLegacy || normalized || schemaMigrated {
			if err := s.persistItemsLocked(s.items); err != nil {
				return err
			}
		}
	} else if normalized || schemaMigrated {
		if err := s.persistItemsLocked(s.items); err != nil {
			return err
		}
	}
	return nil
}

func (s *AttachmentStore) persistItemsLocked(items map[string]models.AttachmentMeta) error {
	if s.indexPath == "" || !s.persist {
		return nil
	}
	payload := struct {
		SchemaVersion int                              `json:"schema_version"`
		Items         map[string]models.AttachmentMeta `json:"items"`
	}{
		SchemaVersion: attachmentIndexSchemaVersion,
		Items:         items,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if s.secret != "" {
		data, err = securestore.Encrypt(s.secret, data)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(s.indexPath, data, 0o600)
}

func (s *AttachmentStore) migrateLegacyFiles(items map[string]models.AttachmentMeta) error {
	if s.secret == "" || len(items) == 0 {
		return nil
	}
	for id := range items {
		path := s.filePath(id)
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if _, derr := securestore.Decrypt(s.secret, raw); derr == nil {
			continue
		} else if !errors.Is(derr, securestore.ErrLegacyData) {
			return derr
		}
		enc, err := securestore.Encrypt(s.secret, raw)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, enc, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *AttachmentStore) touchLastAccess(id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, ok := s.items[id]
	if !ok {
		return ErrAttachmentNotFound
	}
	if !meta.LastAccessAt.Before(at) {
		return nil
	}
	meta.LastAccessAt = at
	nextItems := cloneAttachmentMetaMap(s.items)
	nextItems[id] = meta
	if err := s.persistItemsLocked(nextItems); err != nil {
		return err
	}
	s.items = nextItems
	return nil
}

func (s *AttachmentStore) runGCLocked(now time.Time, imageTTLSeconds, fileTTLSeconds int, dryRun bool) (AttachmentGCReport, error) {
	report := AttachmentGCReport{
		DryRun:         dryRun,
		DeletedByClass: map[string]int{"image": 0, "file": 0},
		DeletedByCause: map[string]int{"expired": 0, "lru": 0},
	}

	toDelete := make(map[string]string)
	tryMarkDelete := func(id, cause string) {
		if _, exists := toDelete[id]; !exists {
			toDelete[id] = cause
		}
	}

	// 1) TTL phase: expire non-pinned blobs.
	for id, meta := range s.items {
		if strings.TrimSpace(meta.PinState) == string(models.AttachmentPinStatePinned) {
			continue
		}
		class := s.normalizedClass(meta)
		ttl := 0
		if class == "image" {
			ttl = imageTTLSeconds
		} else {
			ttl = fileTTLSeconds
		}
		if ttl <= 0 {
			continue
		}
		cutoff := now.Add(-time.Duration(ttl) * time.Second)
		if !meta.CreatedAt.After(cutoff) {
			tryMarkDelete(id, "expired")
		}
	}

	// 2) LRU phase: evict least recently accessed non-pinned blobs until quota satisfied.
	for _, class := range []string{"image", "file"} {
		_, quotaBytes := s.classLimitsLocked(class)
		if quotaBytes <= 0 {
			continue
		}
		usage := s.usageByClassExcludingLocked(class, toDelete)
		targetUsage := quotaBytes
		highWatermarkBytes := (quotaBytes * int64(s.hardCap.HighWatermarkPercent)) / 100
		if highWatermarkBytes <= 0 {
			highWatermarkBytes = quotaBytes
		}
		if usage >= highWatermarkBytes {
			aggressiveTargetBytes := (quotaBytes * int64(s.hardCap.AggressiveTargetPercent)) / 100
			if aggressiveTargetBytes > 0 && aggressiveTargetBytes < targetUsage {
				targetUsage = aggressiveTargetBytes
			}
		}
		if usage <= targetUsage {
			continue
		}
		candidates := s.lruCandidatesLocked(class, toDelete)
		for _, candidateID := range candidates {
			meta, ok := s.items[candidateID]
			if !ok {
				continue
			}
			tryMarkDelete(candidateID, "lru")
			usage -= meta.Size
			if usage <= targetUsage {
				break
			}
		}
	}

	for id, cause := range toDelete {
		meta, ok := s.items[id]
		if !ok {
			continue
		}
		class := s.normalizedClass(meta)
		report.DeletedByClass[class] = report.DeletedByClass[class] + 1
		report.DeletedByCause[cause] = report.DeletedByCause[cause] + 1
		report.DeletedCount++
	}
	if report.DeletedCount == 0 {
		return report, nil
	}
	if dryRun {
		return report, nil
	}
	deletedIDs := make([]string, 0, len(toDelete))
	for id := range toDelete {
		deletedIDs = append(deletedIDs, id)
	}
	if err := s.deleteIDsLocked(deletedIDs); err != nil {
		return AttachmentGCReport{}, err
	}
	return report, nil
}

func (s *AttachmentStore) deleteIDsLocked(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	nextItems := cloneAttachmentMetaMap(s.items)
	for _, id := range ids {
		delete(nextItems, id)
		delete(s.blobs, id)
		if s.persist && strings.TrimSpace(s.dir) != "" {
			if err := os.Remove(s.filePath(id)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	if err := s.persistItemsLocked(nextItems); err != nil {
		return err
	}
	s.items = nextItems
	return nil
}

func (s *AttachmentStore) usageByClassExcludingLocked(class string, excluded map[string]string) int64 {
	var sum int64
	normalizedClass := strings.ToLower(strings.TrimSpace(class))
	for id, meta := range s.items {
		if _, skip := excluded[id]; skip {
			continue
		}
		if s.normalizedClass(meta) == normalizedClass {
			sum += meta.Size
		}
	}
	return sum
}

func (s *AttachmentStore) lruCandidatesLocked(class string, excluded map[string]string) []string {
	type candidate struct {
		id   string
		last time.Time
		size int64
	}
	normalizedClass := strings.ToLower(strings.TrimSpace(class))
	candidates := make([]candidate, 0)
	for id, meta := range s.items {
		if _, skip := excluded[id]; skip {
			continue
		}
		if strings.TrimSpace(meta.PinState) == string(models.AttachmentPinStatePinned) {
			continue
		}
		if s.normalizedClass(meta) != normalizedClass {
			continue
		}
		last := meta.LastAccessAt
		if last.IsZero() {
			last = meta.CreatedAt
		}
		candidates = append(candidates, candidate{id: id, last: last, size: meta.Size})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].last.Equal(candidates[j].last) {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].last.Before(candidates[j].last)
	})
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.id)
	}
	return out
}

func (s *AttachmentStore) normalizedClass(meta models.AttachmentMeta) string {
	class := strings.ToLower(strings.TrimSpace(meta.Class))
	if class == "" {
		class = string(models.ClassifyAttachmentMime(meta.MimeType))
	}
	if class == "image" {
		return "image"
	}
	return "file"
}

func cloneAttachmentMetaMap(in map[string]models.AttachmentMeta) map[string]models.AttachmentMeta {
	out := make(map[string]models.AttachmentMeta, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *AttachmentStore) enforceClassLimitsLocked(class string, size int64) error {
	maxItemBytes, quotaBytes := s.classLimitsLocked(class)
	if maxItemBytes > 0 && size > maxItemBytes {
		return errors.New("attachment exceeds class max item size")
	}
	if quotaBytes > 0 {
		fullCapBytes := (quotaBytes * int64(s.hardCap.FullCapPercent)) / 100
		if fullCapBytes <= 0 {
			fullCapBytes = quotaBytes
		}
		if s.usageByClassLocked(class) >= fullCapBytes {
			if strings.EqualFold(strings.TrimSpace(class), "image") {
				s.hits.ImageFullCapHits++
			} else {
				s.hits.FileFullCapHits++
			}
			return ErrAttachmentHardCapReached
		}
	}
	if quotaBytes > 0 && s.usageByClassLocked(class)+size > quotaBytes {
		targetUsage := quotaBytes - size
		usage := s.usageByClassLocked(class)
		if targetUsage < 0 {
			targetUsage = 0
		}
		if usage > targetUsage {
			toDelete := make([]string, 0)
			for _, candidateID := range s.lruCandidatesLocked(class, map[string]string{}) {
				meta, ok := s.items[candidateID]
				if !ok {
					continue
				}
				toDelete = append(toDelete, candidateID)
				usage -= meta.Size
				if usage <= targetUsage {
					break
				}
			}
			if len(toDelete) > 0 {
				if err := s.deleteIDsLocked(toDelete); err != nil {
					return err
				}
			}
		}
		if s.usageByClassLocked(class)+size > quotaBytes {
			return errors.New("attachment class quota exceeded")
		}
	}
	return nil
}

func (s *AttachmentStore) classLimitsLocked(class string) (maxItemBytes int64, quotaBytes int64) {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "image":
		return s.limits.ImageMaxItemBytes, s.limits.ImageQuotaBytes
	default:
		return s.limits.FileMaxItemBytes, s.limits.FileQuotaBytes
	}
}

func (s *AttachmentStore) usageByClassLocked(class string) int64 {
	normalizedClass := strings.ToLower(strings.TrimSpace(class))
	var sum int64
	for _, meta := range s.items {
		metaClass := strings.ToLower(strings.TrimSpace(meta.Class))
		if metaClass == "" {
			metaClass = string(models.ClassifyAttachmentMime(meta.MimeType))
		}
		if metaClass == normalizedClass {
			sum += meta.Size
		}
	}
	return sum
}

func mbToBytes(mb int) int64 {
	if mb <= 0 {
		return 0
	}
	return int64(mb) * 1024 * 1024
}

func newAttachmentID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "att1_" + hex.EncodeToString(buf), nil
}
