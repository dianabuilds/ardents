package enrollmenttoken

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTokenMalformed        = errors.New("enrollment token is malformed")
	ErrTokenIssuerInvalid    = errors.New("enrollment token issuer is invalid")
	ErrTokenScopeInvalid     = errors.New("enrollment token scope is invalid")
	ErrTokenClaimsInvalid    = errors.New("enrollment token claims are invalid")
	ErrTokenSignatureInvalid = errors.New("enrollment token signature is invalid")
	ErrTokenExpired          = errors.New("enrollment token is expired")
	ErrTokenAlreadyUsed      = errors.New("enrollment token is already used")
)

const RequiredIssuer = "ardents-control-plane"
const RequiredScope = "node.enroll"

type Claims struct {
	TokenID          string    `json:"token_id"`
	IssuedAt         time.Time `json:"issued_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	Scope            string    `json:"scope"`
	SubjectNodeGroup string    `json:"subject_node_group"`
	Issuer           string    `json:"issuer"`
	KeyID            string    `json:"key_id"`
}

type AuditEvent struct {
	EventType string    `json:"event_type"`
	TokenID   string    `json:"token_id,omitempty"`
	Issuer    string    `json:"issuer,omitempty"`
	KeyID     string    `json:"key_id,omitempty"`
	Result    string    `json:"result"`
	Reason    string    `json:"reason,omitempty"`
	At        time.Time `json:"at"`
}

type RedemptionStore interface {
	TryRedeem(tokenID string, at time.Time) (bool, error)
}

type Verifier struct {
	RequiredIssuer string
	RequiredScope  string
	PublicKeys     map[string]ed25519.PublicKey
	Now            func() time.Time
}

func (v Verifier) VerifyAndRedeem(token string, store RedemptionStore) (Claims, AuditEvent, error) {
	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now().UTC()
	}
	claims, payload, signature, err := decodeToken(token)
	if err != nil {
		return Claims{}, rejectedAudit(now, claims, "TOKEN_MALFORMED"), err
	}
	if err := validateClaims(claims, v.RequiredIssuer, v.RequiredScope); err != nil {
		code := "TOKEN_CLAIMS_INVALID"
		if errors.Is(err, ErrTokenIssuerInvalid) {
			code = "TOKEN_ISSUER_INVALID"
		} else if errors.Is(err, ErrTokenScopeInvalid) {
			code = "TOKEN_SCOPE_INVALID"
		}
		return Claims{}, rejectedAudit(now, claims, code), err
	}
	if !claims.ExpiresAt.After(now) {
		return Claims{}, rejectedAudit(now, claims, "TOKEN_EXPIRED"), ErrTokenExpired
	}
	pub, ok := v.PublicKeys[claims.KeyID]
	if !ok || len(pub) != ed25519.PublicKeySize {
		return Claims{}, rejectedAudit(now, claims, "TOKEN_SIGNATURE_INVALID"), ErrTokenSignatureInvalid
	}
	if !ed25519.Verify(pub, payload, signature) {
		return Claims{}, rejectedAudit(now, claims, "TOKEN_SIGNATURE_INVALID"), ErrTokenSignatureInvalid
	}
	if store != nil {
		ok, err := store.TryRedeem(claims.TokenID, now)
		if err != nil {
			return Claims{}, rejectedAudit(now, claims, "TOKEN_REDEEM_FAILED"), err
		}
		if !ok {
			return Claims{}, rejectedAudit(now, claims, "TOKEN_ALREADY_USED"), ErrTokenAlreadyUsed
		}
	}
	return claims, AuditEvent{
		EventType: "enrollment.token.redeemed",
		TokenID:   claims.TokenID,
		Issuer:    claims.Issuer,
		KeyID:     claims.KeyID,
		Result:    "accepted",
		At:        now,
	}, nil
}

func rejectedAudit(at time.Time, claims Claims, reason string) AuditEvent {
	return AuditEvent{
		EventType: "enrollment.token.redeemed",
		TokenID:   claims.TokenID,
		Issuer:    claims.Issuer,
		KeyID:     claims.KeyID,
		Result:    "rejected",
		Reason:    reason,
		At:        at,
	}
}

func validateClaims(claims Claims, requiredIssuer, requiredScope string) error {
	if requiredIssuer == "" {
		requiredIssuer = RequiredIssuer
	}
	if requiredScope == "" {
		requiredScope = RequiredScope
	}
	if strings.TrimSpace(claims.Issuer) != requiredIssuer {
		return ErrTokenIssuerInvalid
	}
	if strings.TrimSpace(claims.Scope) != requiredScope {
		return ErrTokenScopeInvalid
	}
	if strings.TrimSpace(claims.TokenID) == "" ||
		claims.IssuedAt.IsZero() ||
		claims.ExpiresAt.IsZero() ||
		strings.TrimSpace(claims.SubjectNodeGroup) == "" ||
		strings.TrimSpace(claims.KeyID) == "" {
		return ErrTokenClaimsInvalid
	}
	if !claims.ExpiresAt.After(claims.IssuedAt) {
		return ErrTokenClaimsInvalid
	}
	return nil
}

func decodeToken(token string) (Claims, []byte, []byte, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return Claims{}, nil, nil, ErrTokenMalformed
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, nil, nil, ErrTokenMalformed
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, nil, nil, ErrTokenMalformed
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, nil, nil, ErrTokenMalformed
	}
	return claims, payload, signature, nil
}

func EncodeSignedToken(claims Claims, privateKey ed25519.PrivateKey) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	signature := ed25519.Sign(privateKey, payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
