package migrations

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	SupportedMinVersion int64 = 1
	SupportedMaxVersion int64 = 1
)

var (
	ErrMigrationRequired     = errors.New("ERR_MIGRATION_REQUIRED")
	ErrMigrationUnsupported  = errors.New("ERR_MIGRATION_UNSUPPORTED")
	ErrMigrationFailed       = errors.New("ERR_MIGRATION_FAILED")
	ErrMigrationBackupFailed = errors.New("ERR_MIGRATION_BACKUP_FAILED")
)

type DataVersion struct {
	SchemaVersion int64 `json:"schema_version"`
	CreatedAtMs   int64 `json:"created_at_ms"`
	UpdatedAtMs   int64 `json:"updated_at_ms"`
}

func NewVersion(schemaVersion int64, nowMs int64) DataVersion {
	return DataVersion{
		SchemaVersion: schemaVersion,
		CreatedAtMs:   nowMs,
		UpdatedAtMs:   nowMs,
	}
}

func Load(path string) (DataVersion, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return DataVersion{}, err
	}
	var v DataVersion
	if err := json.Unmarshal(b, &v); err != nil {
		return DataVersion{}, err
	}
	return v, nil
}

func Save(path string, v DataVersion) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func CheckCompatibility(v DataVersion) error {
	if v.SchemaVersion < SupportedMinVersion {
		return ErrMigrationRequired
	}
	if v.SchemaVersion > SupportedMaxVersion {
		return ErrMigrationUnsupported
	}
	return nil
}
