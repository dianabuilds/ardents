package privacy

import (
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
		FileTTLSeconds:       180,
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
	if updated.MessageTTLSeconds != 120 || updated.FileTTLSeconds != 180 {
		t.Fatalf("ttl must be preserved, got message=%d file=%d", updated.MessageTTLSeconds, updated.FileTTLSeconds)
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

	updated, err := svc.UpdateStoragePolicy("protected", "zero_retention", 600, 600)
	if err != nil {
		t.Fatalf("update storage policy failed: %v", err)
	}
	if updated.StorageProtection != StorageProtectionProtected {
		t.Fatalf("unexpected storage mode: %q", updated.StorageProtection)
	}
	if updated.ContentRetentionMode != RetentionZeroRetention {
		t.Fatalf("unexpected retention mode: %q", updated.ContentRetentionMode)
	}
	if updated.MessageTTLSeconds != 0 || updated.FileTTLSeconds != 0 {
		t.Fatalf("ttl must be zero for zero retention: %+v", updated)
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
