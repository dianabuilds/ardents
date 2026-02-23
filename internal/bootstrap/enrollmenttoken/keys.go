package enrollmenttoken

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

func ParseIssuerKeys(raw string) (map[string]ed25519.PublicKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("issuer keys are required")
	}
	out := map[string]ed25519.PublicKey{}
	pairs := strings.Split(raw, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid issuer key pair %q", pair)
		}
		keyID := strings.TrimSpace(parts[0])
		pubRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid public key encoding for %q", keyID)
		}
		if len(pubRaw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid public key size for %q", keyID)
		}
		out[keyID] = pubRaw
	}
	return out, nil
}
