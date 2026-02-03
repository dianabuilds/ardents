package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/ed25519util"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

var ErrIdentityInvalid = errors.New("ERR_ID_INVALID")

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
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Identity{}, err
	}
	keyPath := filepath.Join(dir, "identity.key")
	if fileExists(keyPath) {
		return load(keyPath)
	}
	return create(keyPath)
}

func load(path string) (Identity, error) {
	keyPEM, err := os.ReadFile(path) // #nosec G304 -- path is controlled by app dirs.
	if err != nil {
		return Identity{}, err
	}
	edPriv, err := ed25519util.ParsePrivateKeyPEM(keyPEM)
	if err != nil {
		if errors.Is(err, ed25519util.ErrPrivateKeyInvalid) {
			return Identity{}, ErrIdentityInvalid
		}
		return Identity{}, err
	}
	return newIdentity(ed25519util.PublicKey(edPriv), edPriv)
}

func create(path string) (Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	keyPEM, err := ed25519util.EncodePrivateKeyPEM(priv)
	if err != nil {
		return Identity{}, err
	}
	if err := os.WriteFile(path, keyPEM, 0o600); err != nil {
		return Identity{}, err
	}
	return newIdentity(pub, priv)
}

func NewEphemeral() (Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	return newIdentity(pub, priv)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func newIdentity(pub ed25519.PublicKey, priv ed25519.PrivateKey) (Identity, error) {
	id, err := ids.NewIdentityID(pub)
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		ID:          id,
		PrivateKey:  priv,
		PublicKey:   pub,
		CreatedAtMs: timeutil.NowUnixMs(),
	}, nil
}
