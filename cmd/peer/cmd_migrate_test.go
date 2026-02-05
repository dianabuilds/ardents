package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dianabuilds/ardents/internal/core/infra/migrations"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

func TestRunMigrateCreatesBackupAndLog(t *testing.T) {
	home := t.TempDir()
	dirs, err := appdirs.Resolve(home)
	if err != nil {
		t.Fatalf("resolve dirs: %v", err)
	}
	if err := os.MkdirAll(dirs.ConfigDir, 0o750); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.MkdirAll(dirs.DataDir, 0o750); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirs.ConfigDir, "node.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirs.DataDir, "addressbook.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write data: %v", err)
	}

	now := int64(1000)
	if err := runMigrate(home, false, func() int64 { return now }); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}

	v, err := migrations.Load(dirs.VersionPath())
	if err != nil {
		t.Fatalf("load version: %v", err)
	}
	if v.SchemaVersion != migrations.SupportedMaxVersion {
		t.Fatalf("unexpected schema_version: %d", v.SchemaVersion)
	}

	backupDir := filepath.Join(dirs.RunDir, "backup-1000")
	if _, err := os.Stat(filepath.Join(backupDir, "config", "node.json")); err != nil {
		t.Fatalf("backup config missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "data", "addressbook.json")); err != nil {
		t.Fatalf("backup data missing: %v", err)
	}

	logBytes, err := os.ReadFile(filepath.Join(dirs.RunDir, "migrations.jsonl"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "\"status\":\"started\"") || !strings.Contains(logText, "\"status\":\"applied\"") {
		t.Fatalf("log missing entries: %s", logText)
	}
}

func TestRunMigrateUnsupported(t *testing.T) {
	home := t.TempDir()
	dirs, err := appdirs.Resolve(home)
	if err != nil {
		t.Fatalf("resolve dirs: %v", err)
	}
	if err := os.MkdirAll(dirs.DataDir, 0o750); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := migrations.Save(dirs.VersionPath(), migrations.DataVersion{
		SchemaVersion: migrations.SupportedMaxVersion + 1,
		CreatedAtMs:   1,
		UpdatedAtMs:   1,
	}); err != nil {
		t.Fatalf("save version: %v", err)
	}
	if err := runMigrate(home, true, func() int64 { return 1000 }); err != migrations.ErrMigrationUnsupported {
		t.Fatalf("unexpected error: %v", err)
	}
}
