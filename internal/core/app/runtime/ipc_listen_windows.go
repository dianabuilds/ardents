//go:build windows

package runtime

import (
	"net"

	"github.com/natefinch/npipe"
)

func ipcListen() (net.Listener, error) {
	return npipe.Listen(`\\.\pipe\ardents-peer`)
}
