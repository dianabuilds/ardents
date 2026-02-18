package daemonservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DataWipeConsentToken = "I_UNDERSTAND_LOCAL_DATA_WIPE"

func (s *Service) WipeData(consentToken string) (bool, error) {
	if strings.TrimSpace(consentToken) != DataWipeConsentToken {
		return false, errors.New("data wipe requires explicit consent token")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.StopNetworking(stopCtx); err != nil {
		return false, err
	}

	wipeErr := s.wipeContentState()
	if s.identityState != nil {
		if err := s.identityState.Wipe(); err != nil {
			wipeErr = errors.Join(wipeErr, err)
		}
		if dir := s.identityState.StorageDir(); strings.TrimSpace(dir) != "" {
			if err := os.Remove(filepath.Join(dir, "storage.key")); err != nil && !os.IsNotExist(err) {
				wipeErr = errors.Join(wipeErr, err)
			}
		}
	}
	if s.privacyCore != nil {
		if err := s.privacyCore.WipeState(); err != nil {
			wipeErr = errors.Join(wipeErr, err)
		}
	}
	if wipeErr != nil {
		return false, wipeErr
	}
	return true, nil
}
