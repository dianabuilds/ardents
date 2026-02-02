package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/ids"
)

var ErrIdentityInvalid = errors.New("invalid identity")

type Identity struct {
	ID          string
	PrivateKey  ed25519.PrivateKey
	PublicKey   ed25519.PublicKey
	CreatedAtMs int64
}

func LoadOrCreate(dir string) (Identity, error) {
	if dir == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			dir = d.IdentityDir()
		} else {
			dir = filepath.Join("data", "identity")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Identity{}, err
	}
	keyPath := filepath.Join(dir, "identity.key")
	if fileExists(keyPath) {
		return load(keyPath)
	}
	return create(keyPath)
}

func load(path string) (Identity, error) {
	keyPEM, err := os.ReadFile(path)
	if err != nil {
		return Identity{}, err
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "PRIVATE KEY" {
		return Identity{}, ErrIdentityInvalid
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return Identity{}, err
	}
	edPriv, ok := priv.(ed25519.PrivateKey)
	if !ok {
		return Identity{}, ErrIdentityInvalid
	}
	pub := edPriv.Public().(ed25519.PublicKey)
	id, err := ids.NewIdentityID(pub)
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		ID:          id,
		PrivateKey:  edPriv,
		PublicKey:   pub,
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
	}, nil
}

func create(path string) (Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return Identity{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(path, keyPEM, 0o600); err != nil {
		return Identity{}, err
	}
	id, err := ids.NewIdentityID(pub)
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		ID:          id,
		PrivateKey:  priv,
		PublicKey:   pub,
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
	}, nil
}

func NewEphemeral() (Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	id, err := ids.NewIdentityID(pub)
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		ID:          id,
		PrivateKey:  priv,
		PublicKey:   pub,
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
	}, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
