package daemonservice

import (
	"aim-chat/go-backend/internal/bootstrap/enrollmenttoken"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const enrollmentRedeemedStoreFile = "enrollment-redeemed-server.json"

func (s *Service) configureEnrollmentTokenFlow() error {
	keysRaw := strings.TrimSpace(os.Getenv("AIM_ENROLLMENT_ISSUER_KEYS"))
	if keysRaw == "" {
		s.enrollmentKeys = nil
		s.enrollmentStore = nil
		return nil
	}
	keys, err := enrollmenttoken.ParseIssuerKeys(keysRaw)
	if err != nil {
		return err
	}
	s.enrollmentKeys = keys
	path := filepath.Join(s.dataDir, enrollmentRedeemedStoreFile)
	store := enrollmenttoken.NewFileStore(path)
	if err := store.Bootstrap(); err != nil {
		return err
	}
	s.enrollmentStore = store
	return nil
}

func (s *Service) RedeemEnrollmentToken(token string) (enrollmenttoken.Claims, error) {
	if len(s.enrollmentKeys) == 0 || s.enrollmentStore == nil {
		return enrollmenttoken.Claims{}, errors.New("enrollment token flow is not configured")
	}
	verifier := enrollmenttoken.Verifier{
		RequiredIssuer: enrollmenttoken.RequiredIssuer,
		RequiredScope:  enrollmenttoken.RequiredScope,
		PublicKeys:     s.enrollmentKeys,
	}
	claims, audit, err := verifier.VerifyAndRedeem(token, s.enrollmentStore)
	if err != nil {
		s.logger.Warn("enrollment token redeem rejected",
			"event_type", "enrollment.token.redeemed",
			"result", "rejected",
			"reason", err.Error(),
		)
		return enrollmenttoken.Claims{}, err
	}
	s.logger.Info("enrollment token redeemed",
		"event_type", audit.EventType,
		"token_id", audit.TokenID,
		"issuer", audit.Issuer,
		"key_id", audit.KeyID,
		"result", audit.Result,
	)
	return claims, nil
}
