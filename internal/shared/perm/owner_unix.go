//go:build !windows

package perm

import (
	"os"
	"path/filepath"
)

func OpenOwnerOnly(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
