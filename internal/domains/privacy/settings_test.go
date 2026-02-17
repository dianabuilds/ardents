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
	in := PrivacySettings{MessagePrivacyMode: MessagePrivacyMode("invalid")}
	got := NormalizePrivacySettings(in)
	if got.MessagePrivacyMode != DefaultMessagePrivacyMode {
		t.Fatalf("unexpected normalized mode: got=%q want=%q", got.MessagePrivacyMode, DefaultMessagePrivacyMode)
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
