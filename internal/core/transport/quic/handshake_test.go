package quic

import (
	"errors"
	"testing"
)

func TestValidateHelloTimeSkew(t *testing.T) {
	h := Hello{V: HelloVersion, PeerID: "peer_x", TSMs: 0}
	if err := ValidateHello(0, h); !errors.Is(err, ErrHandshakeTimeSkew) {
		t.Fatalf("expected time skew, got %v", err)
	}
}

func TestValidateHelloVersion(t *testing.T) {
	h := Hello{V: 2, PeerID: "peer_x", TSMs: 1}
	if err := ValidateHello(1, h); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected unsupported version, got %v", err)
	}
}

func TestParseQUICAddrInvalid(t *testing.T) {
	if _, err := ParseQUICAddr("bad"); !errors.Is(err, ErrAddrInvalid) {
		t.Fatalf("expected ERR_ADDR_INVALID, got %v", err)
	}
}
