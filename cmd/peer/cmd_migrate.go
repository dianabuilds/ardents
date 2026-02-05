package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dianabuilds/ardents/internal/core/infra/migrations"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type migrationLogEntry struct {
	TsMs        int64  `json:"ts_ms"`
	FromVersion int64  `json:"from_version"`
	ToVersion   int64  `json:"to_version"`
	Status      string `json:"status"`
	ErrorCode   string `json:"error_code,omitempty"`
}

func migrateCmd(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	noBackup := fs.Bool("no-backup", false, "skip backup creation")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if err := runMigrate(*home, *noBackup, timeutil.NowUnixMs); err != nil {
		fatal(err)
	}
}

func runMigrate(home string, noBackup bool, nowFn func() int64) error {
	dirs, err := resolveDirs(home)
	if err != nil {
		return err
	}
	nowMs := nowFn()
	current, err := loadVersionOrZero(dirs)
	if err != nil {
		return err
	}
	if current.SchemaVersion > migrations.SupportedMaxVersion {
		return migrations.ErrMigrationUnsupported
	}
	if current.SchemaVersion >= migrations.SupportedMaxVersion {
		fmt.Println("already up to date")
		return nil
	}

	fromVersion := current.SchemaVersion
	toVersion := migrations.SupportedMaxVersion
	if err := writeMigrationLog(dirs, migrationLogEntry{
		TsMs:        nowMs,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Status:      "started",
	}); err != nil {
		return migrations.ErrMigrationFailed
	}

	if !noBackup {
		if err := createBackup(dirs, nowMs); err != nil {
			_ = writeMigrationLog(dirs, migrationLogEntry{
				TsMs:        nowFn(),
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				Status:      "failed",
				ErrorCode:   migrations.ErrMigrationBackupFailed.Error(),
			})
			return migrations.ErrMigrationBackupFailed
		}
	}

	if err := applyMigrations(dirs, current, toVersion, nowMs); err != nil {
		_ = writeMigrationLog(dirs, migrationLogEntry{
			TsMs:        nowFn(),
			FromVersion: fromVersion,
			ToVersion:   toVersion,
			Status:      "failed",
			ErrorCode:   migrations.ErrMigrationFailed.Error(),
		})
		return migrations.ErrMigrationFailed
	}

	if err := writeMigrationLog(dirs, migrationLogEntry{
		TsMs:        nowFn(),
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Status:      "applied",
	}); err != nil {
		return migrations.ErrMigrationFailed
	}

	fmt.Println("migration applied:", fromVersion, "->", toVersion)
	return nil
}

func loadVersionOrZero(dirs appdirs.Dirs) (migrations.DataVersion, error) {
	v, err := migrations.Load(dirs.VersionPath())
	if err == nil {
		return v, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return migrations.DataVersion{SchemaVersion: 0}, nil
	}
	return migrations.DataVersion{}, err
}

func applyMigrations(dirs appdirs.Dirs, current migrations.DataVersion, toVersion int64, nowMs int64) error {
	if toVersion < migrations.SupportedMinVersion || toVersion > migrations.SupportedMaxVersion {
		return migrations.ErrMigrationUnsupported
	}
	if current.SchemaVersion >= toVersion {
		return nil
	}
	createdAt := current.CreatedAtMs
	if createdAt <= 0 {
		createdAt = nowMs
	}
	v := migrations.DataVersion{
		SchemaVersion: toVersion,
		CreatedAtMs:   createdAt,
		UpdatedAtMs:   nowMs,
	}
	return migrations.Save(dirs.VersionPath(), v)
}

func createBackup(dirs appdirs.Dirs, nowMs int64) error {
	if err := os.MkdirAll(dirs.RunDir, 0o750); err != nil {
		return err
	}
	backupDir := filepath.Join(dirs.RunDir, fmt.Sprintf("backup-%d", nowMs))
	cfgDst := filepath.Join(backupDir, "config")
	dataDst := filepath.Join(backupDir, "data")
	if err := copyDir(dirs.ConfigDir, cfgDst); err != nil {
		return err
	}
	if err := copyDir(dirs.DataDir, dataDst); err != nil {
		return err
	}
	return nil
}

func writeMigrationLog(dirs appdirs.Dirs, entry migrationLogEntry) error {
	if err := os.MkdirAll(dirs.RunDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(dirs.RunDir, "migrations.jsonl")
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func copyDir(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return migrations.ErrMigrationBackupFailed
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
