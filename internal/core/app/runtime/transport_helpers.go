package runtime

import "errors"

var ErrDialerUnavailable = errors.New("ERR_DIALER_UNAVAILABLE")

func stripSchemeLocal(addr string) string {
	const prefix = "quic://"
	if len(addr) >= len(prefix) && addr[:len(prefix)] == prefix {
		return addr[len(prefix):]
	}
	return addr
}
