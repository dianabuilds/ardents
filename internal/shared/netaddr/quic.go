package netaddr

import "strings"

// StripQUICScheme removes "quic://" prefix from an address string.
// We keep this helper in shared to avoid subtle behavior drift across CLIs and transports.
func StripQUICScheme(addr string) string {
	const prefix = "quic://"
	if strings.HasPrefix(addr, prefix) {
		return strings.TrimPrefix(addr, prefix)
	}
	return addr
}
