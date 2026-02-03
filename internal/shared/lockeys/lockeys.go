package lockeys

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/crypto/curve25519"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

var ErrKeyInvalid = errors.New("ERR_LKEY_INVALID")

type Keypair struct {
	V       uint64 `cbor:"v"`
	Public  []byte `cbor:"public"`
	Private []byte `cbor:"private"`
}

func Load(dir string, serviceID string) (Keypair, error) {
	if dir == "" || serviceID == "" {
		return Keypair{}, ErrKeyInvalid
	}
	path := filepath.Join(dir, serviceID+".key")
	data, err := os.ReadFile(path) // #nosec G304 -- path is controlled by app dirs.
	if err != nil {
		return Keypair{}, err
	}
	var k Keypair
	if err := codec.Unmarshal(data, &k); err != nil {
		return Keypair{}, err
	}
	if k.V != 1 || len(k.Public) != 32 || len(k.Private) != 32 {
		return Keypair{}, ErrKeyInvalid
	}
	return k, nil
}

func LoadOrCreate(dir string, serviceID string) (Keypair, error) {
	if dir == "" || serviceID == "" {
		return Keypair{}, ErrKeyInvalid
	}
	if k, err := Load(dir, serviceID); err == nil {
		return k, nil
	}
	return create(dir, serviceID)
}

func Rotate(dir string, serviceID string) (Keypair, error) {
	if dir == "" || serviceID == "" {
		return Keypair{}, ErrKeyInvalid
	}
	return create(dir, serviceID)
}

func create(dir string, serviceID string) (Keypair, error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return Keypair{}, err
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return Keypair{}, err
	}
	k := Keypair{V: 1, Public: pub, Private: priv}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Keypair{}, err
	}
	data, err := codec.Marshal(k)
	if err != nil {
		return Keypair{}, err
	}
	path := filepath.Join(dir, serviceID+".key")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return Keypair{}, err
	}
	return k, nil
}
