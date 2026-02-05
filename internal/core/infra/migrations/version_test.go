package migrations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "version.json")
	v := DataVersion{
		SchemaVersion: 1,
		CreatedAtMs:   1000,
		UpdatedAtMs:   2000,
	}
	if err := Save(path, v); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != v {
		t.Fatalf("unexpected value: %+v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	}
}

func TestCheckCompatibility(t *testing.T) {
	cases := []struct {
		name string
		v    DataVersion
		want error
	}{
		{"ok", DataVersion{SchemaVersion: 1}, nil},
		{"too_low", DataVersion{SchemaVersion: 0}, ErrMigrationRequired},
		{"too_high", DataVersion{SchemaVersion: 2}, ErrMigrationUnsupported},
	}
	for _, tc := range cases {
		if err := CheckCompatibility(tc.v); err != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, err, tc.want)
		}
	}
}
