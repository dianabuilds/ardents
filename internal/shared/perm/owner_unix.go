//go:build !windows

package perm

import (
	"errors"
	"os"
	"path/filepath"
)

func OpenOwnerOnly(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	// Secrets and tokens must live in owner-only directories to prevent
	// listing / traversal by other users on multi-user systems.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// MkdirAll doesn't change permissions for existing dirs; enforce anyway.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, errors.New("ERR_GATEWAY_UNAUTHORIZED")
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

func EnsureOwnerOnly(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if dirInfo.Mode().Perm()&0o077 != 0 {
		return errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	return nil
}
