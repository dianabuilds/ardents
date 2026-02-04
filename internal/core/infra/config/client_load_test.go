package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClient_ToleratesUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.json")
	// Include UTF-8 BOM to simulate common Windows editors.
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"v":1,"bootstrap_peers":[{"peer_id":"p","addrs":["127.0.0.1:1"]}],"trusted_identities":[],"refresh_ms":60000,"reseed":{"enabled":false,"network_id":"ardents.mainnet","urls":[],"authorities":[]}}`)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadClient(path); err != nil {
		t.Fatalf("LoadClient: %v", err)
	}
}

func TestLoadClient_InvalidRefreshMs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.json")
	if err := os.WriteFile(path, []byte(`{"v":1,"bootstrap_peers":[{"peer_id":"p","addrs":["127.0.0.1:1"]}],"trusted_identities":[],"refresh_ms":1,"reseed":{"enabled":false,"network_id":"ardents.mainnet","urls":[],"authorities":[]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadClient(path); err == nil || !errors.Is(err, ErrClientConfigInvalid) {
		t.Fatalf("expected ErrClientConfigInvalid, got %v", err)
	}
}
