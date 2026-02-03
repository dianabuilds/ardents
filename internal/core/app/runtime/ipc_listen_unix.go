//go:build !windows

package runtime

import (
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

func ipcListen() (net.Listener, error) {
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return nil, err
	}
	socketPath := filepath.Join(dirs.RunDir, "peer.sock")
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = ln.Close()
		return nil, errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	return ln, nil
}
