package privacy

import (
	"errors"
	"testing"
)

func TestDefaultPrivacySettings(t *testing.T) {
	got := DefaultPrivacySettings()
	if got.MessagePrivacyMode != DefaultMessagePrivacyMode {
		t.Fatalf("unexpected default privacy mode: got=%q want=%q", got.MessagePrivacyMode, DefaultMessagePrivacyMode)
	}
	if got.StorageProtection != DefaultStorageProtectionMode {
		t.Fatalf("unexpected default storage protection: got=%q want=%q", got.StorageProtection, DefaultStorageProtectionMode)
	}
	if got.ContentRetentionMode != DefaultContentRetentionMode {
		t.Fatalf("unexpected default retention mode: got=%q want=%q", got.ContentRetentionMode, DefaultContentRetentionMode)
	}
}

func TestMessagePrivacyModeValid(t *testing.T) {
	cases := []struct {
		name string
		mode MessagePrivacyMode
		want bool
	}{
		{name: "contacts_only", mode: MessagePrivacyContactsOnly, want: true},
		{name: "requests", mode: MessagePrivacyRequests, want: true},
		{name: "everyone", mode: MessagePrivacyEveryone, want: true},
		{name: "invalid", mode: MessagePrivacyMode("unknown"), want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mode.Valid(); got != tc.want {
				t.Fatalf("valid mismatch for %q: got=%v want=%v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestNormalizePrivacySettings(t *testing.T) {
	in := PrivacySettings{
		MessagePrivacyMode:   MessagePrivacyMode("invalid"),
		StorageProtection:    StorageProtectionMode("bad"),
		ContentRetentionMode: ContentRetentionMode("bad"),
		MessageTTLSeconds:    -1,
		ImageTTLSeconds:      -2,
		FileTTLSeconds:       -4,
		ImageQuotaMB:         -3,
		FileQuotaMB:          -5,
		ImageMaxItemSizeMB:   -6,
		FileMaxItemSizeMB:    -7,
	}
	got := NormalizePrivacySettings(in)
	if got.MessagePrivacyMode != DefaultMessagePrivacyMode {
		t.Fatalf("unexpected normalized mode: got=%q want=%q", got.MessagePrivacyMode, DefaultMessagePrivacyMode)
	}
	if got.StorageProtection != DefaultStorageProtectionMode {
		t.Fatalf("unexpected normalized storage mode: got=%q want=%q", got.StorageProtection, DefaultStorageProtectionMode)
	}
	if got.ContentRetentionMode != DefaultContentRetentionMode {
		t.Fatalf("unexpected normalized retention mode: got=%q want=%q", got.ContentRetentionMode, DefaultContentRetentionMode)
	}
	if got.MessageTTLSeconds != 0 || got.ImageTTLSeconds != 0 || got.FileTTLSeconds != 0 {
		t.Fatalf("ttl must be reset in persistent mode, got message=%d image=%d file=%d", got.MessageTTLSeconds, got.ImageTTLSeconds, got.FileTTLSeconds)
	}
	if got.ImageQuotaMB != 0 || got.FileQuotaMB != 0 || got.ImageMaxItemSizeMB != 0 || got.FileMaxItemSizeMB != 0 {
		t.Fatalf("limits must be normalized to zero, got=%+v", got)
	}
}

func TestParseMessagePrivacyMode(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		got, err := ParseMessagePrivacyMode(string(MessagePrivacyEveryone))
		if err != nil {
			t.Fatalf("parse mode failed: %v", err)
		}
		if got != MessagePrivacyEveryone {
			t.Fatalf("unexpected mode: got=%q want=%q", got, MessagePrivacyEveryone)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := ParseMessagePrivacyMode("invalid")
		if err == nil {
			t.Fatal("expected invalid mode error")
		}
		if !errors.Is(err, ErrInvalidMessagePrivacyMode) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestNormalizePrivacySettingsEphemeralDefaultsTTL(t *testing.T) {
	in := PrivacySettings{
		MessagePrivacyMode:   MessagePrivacyEveryone,
		StorageProtection:    StorageProtectionProtected,
		ContentRetentionMode: RetentionEphemeral,
	}
	got := NormalizePrivacySettings(in)
	if got.MessageTTLSeconds != DefaultEphemeralMessageTTLSeconds {
		t.Fatalf("unexpected message ttl: got=%d want=%d", got.MessageTTLSeconds, DefaultEphemeralMessageTTLSeconds)
	}
	if got.FileTTLSeconds != DefaultEphemeralFileTTLSeconds {
		t.Fatalf("unexpected file ttl: got=%d want=%d", got.FileTTLSeconds, DefaultEphemeralFileTTLSeconds)
	}
}

func TestParseStoragePolicy(t *testing.T) {
	got, err := ParseStoragePolicy("protected", "zero_retention", 100, 150, 200, 512, 256, 25, 10)
	if err != nil {
		t.Fatalf("parse storage policy failed: %v", err)
	}
	if got.StorageProtection != StorageProtectionProtected {
		t.Fatalf("unexpected storage mode: %q", got.StorageProtection)
	}
	if got.ContentRetentionMode != RetentionZeroRetention {
		t.Fatalf("unexpected retention mode: %q", got.ContentRetentionMode)
	}
	if got.MessageTTLSeconds != 0 || got.ImageTTLSeconds != 0 || got.FileTTLSeconds != 0 {
		t.Fatalf("ttl must be zero in zero retention: %+v", got)
	}
	if got.ImageQuotaMB != 512 || got.FileQuotaMB != 256 || got.ImageMaxItemSizeMB != 25 || got.FileMaxItemSizeMB != 10 {
		t.Fatalf("unexpected class policy values: %+v", got)
	}
}

func TestResolveStoragePolicyForScopePriorityMatrix(t *testing.T) {
	settings := NormalizePrivacySettings(PrivacySettings{
		MessagePrivacyMode:    MessagePrivacyEveryone,
		StorageProtection:     StorageProtectionStandard,
		ContentRetentionMode:  RetentionEphemeral,
		MessageTTLSeconds:     300,
		ImageTTLSeconds:       300,
		FileTTLSeconds:        300,
		StorageScopeOverrides: map[string]StoragePolicyOverride{},
	})
	key, err := ScopeOverrideKey("group", "g1")
	if err != nil {
		t.Fatalf("scope key error: %v", err)
	}
	settings.StorageScopeOverrides[key] = StoragePolicyOverride{
		StorageProtection:    StorageProtectionProtected,
		ContentRetentionMode: RetentionPersistent,
	}

	resolved, err := ResolveStoragePolicyForScope(settings, "group", "g1", false)
	if err != nil {
		t.Fatalf("resolve scoped policy failed: %v", err)
	}
	if resolved.StorageProtection != StorageProtectionProtected || resolved.ContentRetentionMode != RetentionPersistent {
		t.Fatalf("scope override must win, got %+v", resolved)
	}

	userDefaultResolved, err := ResolveStoragePolicyForScope(settings, "group", "g2", false)
	if err != nil {
		t.Fatalf("resolve user default failed: %v", err)
	}
	if userDefaultResolved.StorageProtection != StorageProtectionStandard || userDefaultResolved.ContentRetentionMode != RetentionEphemeral {
		t.Fatalf("user default fallback mismatch: %+v", userDefaultResolved)
	}
}

func TestResolveStoragePolicyInfiniteTTLFailClosedOnNonPinned(t *testing.T) {
	settings := NormalizePrivacySettings(PrivacySettings{
		StorageScopeOverrides: map[string]StoragePolicyOverride{},
	})
	key, err := ScopeOverrideKey("chat", "c1")
	if err != nil {
		t.Fatalf("scope key error: %v", err)
	}
	settings.StorageScopeOverrides[key] = StoragePolicyOverride{
		StorageProtection:      StorageProtectionProtected,
		ContentRetentionMode:   RetentionPersistent,
		InfiniteTTL:            true,
		PinRequiredForInfinite: true,
	}

	if _, err := ResolveStoragePolicyForScope(settings, "chat", "c1", false); !errors.Is(err, ErrInfiniteTTLRequiresPinned) {
		t.Fatalf("expected ErrInfiniteTTLRequiresPinned, got: %v", err)
	}
	if _, err := ResolveStoragePolicyForScope(settings, "chat", "c1", true); err != nil {
		t.Fatalf("expected pinned resolve success, got: %v", err)
	}
}
