package privacy

import (
	"errors"
	"testing"
)

type fakePrivacyStore struct {
	settings PrivacySettings
}

func (f *fakePrivacyStore) Configure(_, _ string) {}
func (f *fakePrivacyStore) Bootstrap() (PrivacySettings, error) {
	return f.settings, nil
}
func (f *fakePrivacyStore) Persist(settings PrivacySettings) error {
	f.settings = settings
	return nil
}

type fakeBlocklistStore struct {
	list Blocklist
}

func (f *fakeBlocklistStore) Configure(_, _ string) {}
func (f *fakeBlocklistStore) Bootstrap() (Blocklist, error) {
	return f.list, nil
}
func (f *fakeBlocklistStore) Persist(list Blocklist) error {
	f.list = list
	return nil
}

func TestServiceUpdatePrivacySettings(t *testing.T) {
	bl, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	svc := NewService(
		&fakePrivacyStore{settings: DefaultPrivacySettings()},
		&fakeBlocklistStore{list: bl},
		nil,
	)
	svc.SetState(DefaultPrivacySettings(), bl)

	updated, err := svc.UpdatePrivacySettings(string(MessagePrivacyRequests))
	if err != nil {
		t.Fatalf("update privacy settings failed: %v", err)
	}
	if updated.MessagePrivacyMode != MessagePrivacyRequests {
		t.Fatalf("unexpected mode: got=%q want=%q", updated.MessagePrivacyMode, MessagePrivacyRequests)
	}
	if svc.CurrentMode() != MessagePrivacyRequests {
		t.Fatalf("current mode mismatch: got=%q want=%q", svc.CurrentMode(), MessagePrivacyRequests)
	}
}

func TestServiceUpdatePrivacySettingsPreservesStoragePolicy(t *testing.T) {
	bl, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	initial := NormalizePrivacySettings(PrivacySettings{
		MessagePrivacyMode:   MessagePrivacyEveryone,
		StorageProtection:    StorageProtectionProtected,
		ContentRetentionMode: RetentionEphemeral,
		MessageTTLSeconds:    120,
		ImageTTLSeconds:      150,
		FileTTLSeconds:       180,
		ImageQuotaMB:         512,
		FileQuotaMB:          256,
		ImageMaxItemSizeMB:   25,
		FileMaxItemSizeMB:    10,
	})
	svc := NewService(
		&fakePrivacyStore{settings: initial},
		&fakeBlocklistStore{list: bl},
		nil,
	)
	svc.SetState(initial, bl)

	updated, err := svc.UpdatePrivacySettings(string(MessagePrivacyRequests))
	if err != nil {
		t.Fatalf("update privacy settings failed: %v", err)
	}
	if updated.StorageProtection != StorageProtectionProtected {
		t.Fatalf("storage protection must be preserved, got=%q", updated.StorageProtection)
	}
	if updated.ContentRetentionMode != RetentionEphemeral {
		t.Fatalf("retention mode must be preserved, got=%q", updated.ContentRetentionMode)
	}
	if updated.MessageTTLSeconds != 120 || updated.ImageTTLSeconds != 150 || updated.FileTTLSeconds != 180 {
		t.Fatalf("ttl must be preserved, got message=%d image=%d file=%d", updated.MessageTTLSeconds, updated.ImageTTLSeconds, updated.FileTTLSeconds)
	}
	if updated.ImageQuotaMB != 512 || updated.FileQuotaMB != 256 || updated.ImageMaxItemSizeMB != 25 || updated.FileMaxItemSizeMB != 10 {
		t.Fatalf("class policy must be preserved, got %+v", updated)
	}
}

func TestServiceUpdateStoragePolicy(t *testing.T) {
	bl, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	svc := NewService(
		&fakePrivacyStore{settings: DefaultPrivacySettings()},
		&fakeBlocklistStore{list: bl},
		nil,
	)
	svc.SetState(DefaultPrivacySettings(), bl)

	updated, err := svc.UpdateStoragePolicy("protected", "zero_retention", 600, 600, 600, 512, 256, 25, 10)
	if err != nil {
		t.Fatalf("update storage policy failed: %v", err)
	}
	if updated.StorageProtection != StorageProtectionProtected {
		t.Fatalf("unexpected storage mode: %q", updated.StorageProtection)
	}
	if updated.ContentRetentionMode != RetentionZeroRetention {
		t.Fatalf("unexpected retention mode: %q", updated.ContentRetentionMode)
	}
	if updated.MessageTTLSeconds != 0 || updated.ImageTTLSeconds != 0 || updated.FileTTLSeconds != 0 {
		t.Fatalf("ttl must be zero for zero retention: %+v", updated)
	}
	if updated.ImageQuotaMB != 512 || updated.FileQuotaMB != 256 || updated.ImageMaxItemSizeMB != 25 || updated.FileMaxItemSizeMB != 10 {
		t.Fatalf("unexpected class policy values: %+v", updated)
	}
}

func TestServiceBlocklistRoundtrip(t *testing.T) {
	bl, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	svc := NewService(
		&fakePrivacyStore{settings: DefaultPrivacySettings()},
		&fakeBlocklistStore{list: bl},
		nil,
	)
	svc.SetState(DefaultPrivacySettings(), bl)

	id := "aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF"
	list, err := svc.AddToBlocklist(id)
	if err != nil {
		t.Fatalf("add to blocklist failed: %v", err)
	}
	if len(list) != 1 || list[0] != id {
		t.Fatalf("unexpected blocklist after add: %+v", list)
	}
	if !svc.IsBlockedSender(id) {
		t.Fatal("sender must be blocked after add")
	}

	list, err = svc.RemoveFromBlocklist(id)
	if err != nil {
		t.Fatalf("remove from blocklist failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("unexpected blocklist after remove: %+v", list)
	}
	if svc.IsBlockedSender(id) {
		t.Fatal("sender must not be blocked after remove")
	}
}

func TestServiceStorageScopeOverrideRoundtripAndResolve(t *testing.T) {
	bl, err := NewBlocklist(nil)
	if err != nil {
		t.Fatalf("new blocklist failed: %v", err)
	}
	initial := NormalizePrivacySettings(PrivacySettings{
		StorageProtection:    StorageProtectionStandard,
		ContentRetentionMode: RetentionEphemeral,
		MessageTTLSeconds:    120,
	})
	svc := NewService(
		&fakePrivacyStore{settings: initial},
		&fakeBlocklistStore{list: bl},
		nil,
	)
	svc.SetState(initial, bl)

	override, err := svc.SetStorageScopeOverride("group", "g1", StoragePolicyOverride{
		StorageProtection:      StorageProtectionProtected,
		ContentRetentionMode:   RetentionPersistent,
		InfiniteTTL:            true,
		PinRequiredForInfinite: true,
	})
	if err != nil {
		t.Fatalf("set scope override failed: %v", err)
	}
	if !override.InfiniteTTL || !override.PinRequiredForInfinite {
		t.Fatalf("unexpected override flags: %+v", override)
	}

	stored, exists, err := svc.GetStorageScopeOverride("group", "g1")
	if err != nil {
		t.Fatalf("get scope override failed: %v", err)
	}
	if !exists || !stored.InfiniteTTL {
		t.Fatalf("expected stored override, exists=%v override=%+v", exists, stored)
	}

	if _, err := svc.ResolveStoragePolicy("group", "g1", false); !errors.Is(err, ErrInfiniteTTLRequiresPinned) {
		t.Fatalf("expected ErrInfiniteTTLRequiresPinned, got: %v", err)
	}
	resolved, err := svc.ResolveStoragePolicy("group", "g1", true)
	if err != nil {
		t.Fatalf("resolve scope policy for pinned failed: %v", err)
	}
	if resolved.StorageProtection != StorageProtectionProtected || resolved.ContentRetentionMode != RetentionPersistent {
		t.Fatalf("unexpected resolved scoped policy: %+v", resolved)
	}

	fallback, err := svc.ResolveStoragePolicy("group", "g2", false)
	if err != nil {
		t.Fatalf("resolve fallback policy failed: %v", err)
	}
	if fallback.StorageProtection != StorageProtectionStandard || fallback.ContentRetentionMode != RetentionEphemeral {
		t.Fatalf("unexpected fallback policy: %+v", fallback)
	}

	removed, err := svc.RemoveStorageScopeOverride("group", "g1")
	if err != nil {
		t.Fatalf("remove scope override failed: %v", err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}
}
