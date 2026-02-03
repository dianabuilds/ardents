//go:build windows

package main

import (
	"net"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/natefinch/npipe"
)

func ipcDial(dirs appdirs.Dirs) (net.Conn, error) {
	_ = dirs
	return npipe.DialTimeout(`\\.\pipe\ardents-peer`, 2*time.Second)
}
