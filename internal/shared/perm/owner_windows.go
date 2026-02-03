//go:build windows

package perm

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func OpenOwnerOnly(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := restrictToCurrentUser(path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func restrictToCurrentUser(path string) error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer func() { _ = token.Close() }()

	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	sddl := "D:P(A;;FA;;;" + user.User.Sid.String() + ")"
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func EnsureOwnerOnly(path string) error {
	if err := restrictToCurrentUser(path); err != nil {
		return errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	return nil
}
