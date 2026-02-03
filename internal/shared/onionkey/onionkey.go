package onionkey

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/crypto/curve25519"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

var ErrKeyInvalid = errors.New("invalid onion key")

type Keypair struct {
	Private []byte
	Public  []byte
}

func LoadOrCreate(dir string) (Keypair, error) {
	if dir == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			dir = d.KeysDir()
		} else {
			dir = filepath.Join("data", "keys")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Keypair{}, err
	}
	keyPath := filepath.Join(dir, "onion.key")
	if fileExists(keyPath) {
		return load(keyPath)
	}
	return create(keyPath)
}

func load(path string) (Keypair, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Keypair{}, err
	}
	if len(data) != 32 {
		return Keypair{}, ErrKeyInvalid
	}
	priv := make([]byte, 32)
	copy(priv, data)
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return Keypair{}, err
	}
	return Keypair{Private: priv, Public: pub}, nil
}

func create(path string) (Keypair, error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return Keypair{}, err
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return Keypair{}, err
	}
	if err := os.WriteFile(path, priv, 0o600); err != nil {
		return Keypair{}, err
	}
	return Keypair{Private: priv, Public: pub}, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
