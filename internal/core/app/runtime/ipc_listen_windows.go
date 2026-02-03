//go:build windows

package runtime

import (
	"errors"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func ipcListen() (net.Listener, error) {
	sddl, err := currentUserPipeSDDL()
	if err != nil {
		return nil, errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	ln, err := winio.ListenPipe(`\\.\pipe\ardents-peer`, &winio.PipeConfig{
		SecurityDescriptor: sddl,
		MessageMode:        false,
	})
	if err != nil {
		return nil, err
	}
	return ln, nil
}

func currentUserPipeSDDL() (string, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return "", err
	}
	defer func() { _ = token.Close() }()

	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	// Restrict pipe access to current user only.
	return "D:P(A;;GA;;;" + user.User.Sid.String() + ")", nil
}
