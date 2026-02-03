//go:build !windows

package main

import (
	"net"
	"path/filepath"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

func ipcDial(dirs appdirs.Dirs) (net.Conn, error) {
	socketPath := filepath.Join(dirs.RunDir, "peer.sock")
	return net.Dial("unix", socketPath)
}
