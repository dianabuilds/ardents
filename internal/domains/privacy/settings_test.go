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
		FileTTLSeconds:       -4,
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
	if got.MessageTTLSeconds != 0 || got.FileTTLSeconds != 0 {
		t.Fatalf("ttl must be reset in persistent mode, got message=%d file=%d", got.MessageTTLSeconds, got.FileTTLSeconds)
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
	got, err := ParseStoragePolicy("protected", "zero_retention", 100, 200)
	if err != nil {
		t.Fatalf("parse storage policy failed: %v", err)
	}
	if got.StorageProtection != StorageProtectionProtected {
		t.Fatalf("unexpected storage mode: %q", got.StorageProtection)
	}
	if got.ContentRetentionMode != RetentionZeroRetention {
		t.Fatalf("unexpected retention mode: %q", got.ContentRetentionMode)
	}
	if got.MessageTTLSeconds != 0 || got.FileTTLSeconds != 0 {
		t.Fatalf("ttl must be zero in zero retention: %+v", got)
	}
}
