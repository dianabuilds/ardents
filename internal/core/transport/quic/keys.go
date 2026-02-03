package quic

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/ed25519util"
)

var ErrKeyMaterialInvalid = errors.New("ERR_KEY_MATERIAL_INVALID")

type KeyMaterial struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	TLSCert    tls.Certificate
}

func LoadOrCreateKeyMaterial(dir string) (KeyMaterial, error) {
	if dir == "" {
		if d, err := appdirs.Resolve(""); err == nil {
			dir = d.KeysDir()
		} else {
			dir = filepath.Join("data", "keys")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return KeyMaterial{}, err
	}
	keyPath := filepath.Join(dir, "peer.key")
	crtPath := filepath.Join(dir, "peer.crt")

	if fileExists(keyPath) && fileExists(crtPath) {
		return loadKeyMaterial(keyPath, crtPath)
	}
	return createKeyMaterial(keyPath, crtPath)
}

func loadKeyMaterial(keyPath, crtPath string) (KeyMaterial, error) {
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return KeyMaterial{}, err
	}
	edPriv, err := ed25519util.ParsePrivateKeyPEM(keyPEM)
	if err != nil {
		if errors.Is(err, ed25519util.ErrPrivateKeyInvalid) {
			return KeyMaterial{}, ErrKeyMaterialInvalid
		}
		return KeyMaterial{}, err
	}
	crtPEM, err := os.ReadFile(crtPath)
	if err != nil {
		return KeyMaterial{}, err
	}
	cert, err := tls.X509KeyPair(crtPEM, keyPEM)
	if err != nil {
		return KeyMaterial{}, err
	}
	return buildKeyMaterial(edPriv, ed25519util.PublicKey(edPriv), cert), nil
}

func createKeyMaterial(keyPath, crtPath string) (KeyMaterial, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyMaterial{}, err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		return KeyMaterial{}, err
	}
	keyPEM, err := ed25519util.EncodePrivateKeyPEM(priv)
	if err != nil {
		return KeyMaterial{}, err
	}
	crtPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return KeyMaterial{}, err
	}
	if err := os.WriteFile(crtPath, crtPEM, 0o600); err != nil {
		return KeyMaterial{}, err
	}

	cert, err := tls.X509KeyPair(crtPEM, keyPEM)
	if err != nil {
		return KeyMaterial{}, err
	}
	return buildKeyMaterial(priv, pub, cert), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func buildKeyMaterial(priv ed25519.PrivateKey, pub ed25519.PublicKey, cert tls.Certificate) KeyMaterial {
	return KeyMaterial{
		PrivateKey: priv,
		PublicKey:  pub,
		TLSCert:    cert,
	}
}
