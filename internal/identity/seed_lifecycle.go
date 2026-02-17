package identity

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"

	"github.com/tyler-smith/go-bip39"
)

var (
	ErrInvalidMnemonic  = errors.New("invalid mnemonic")
	ErrInvalidPassword  = errors.New("invalid password")
	ErrSeedNotAvailable = errors.New("seed is not available")
	ErrPasswordRequired = errors.New("password is required")
	ErrMnemonicRequired = errors.New("mnemonic is required")
	ErrIdentityInit     = errors.New("identity initialization failed")
	ErrPasswordLocked   = errors.New("password attempts are temporarily locked")
)

type SeedManager struct {
	mu             sync.RWMutex
	envelope       *EncryptedSeedEnvelope
	failedAttempts int
	lockedUntil    time.Time
	now            func() time.Time
}

func NewSeedManager() *SeedManager {
	return &SeedManager{now: time.Now}
}

func newSeedManagerWithClock(now func() time.Time) *SeedManager {
	return &SeedManager{now: now}
}

func (s *SeedManager) Create(password string) (mnemonic string, keys *DerivedKeys, err error) {
	if strings.TrimSpace(password) == "" {
		return "", nil, ErrPasswordRequired
	}
	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return "", nil, err
	}
	mnemonic, err = bip39.NewMnemonic(entropy)
	if err != nil {
		return "", nil, err
	}
	return s.Import(mnemonic, password)
}

func (s *SeedManager) Import(mnemonic, password string) (normalizedMnemonic string, keys *DerivedKeys, err error) {
	mnemonic = strings.TrimSpace(mnemonic)
	if mnemonic == "" {
		return "", nil, ErrMnemonicRequired
	}
	if strings.TrimSpace(password) == "" {
		return "", nil, ErrPasswordRequired
	}
	if !bip39.IsMnemonicValid(mnemonic) {
		return "", nil, ErrInvalidMnemonic
	}

	seedBytes := bip39.NewSeed(mnemonic, "")
	keys, err = DeriveKeys(seedBytes)
	if err != nil {
		return "", nil, err
	}
	env, err := EncryptSeed([]byte(mnemonic), []byte(password))
	if err != nil {
		return "", nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.envelope = env
	return mnemonic, keys, nil
}

func (s *SeedManager) Export(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", ErrPasswordRequired
	}

	s.mu.Lock()
	env := s.envelope
	if err := s.ensureUnlocked(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	s.mu.Unlock()
	if env == nil {
		return "", ErrSeedNotAvailable
	}

	plaintext, err := DecryptSeed(env, []byte(password))
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.onFailedPasswordAttempt()
		return "", ErrInvalidPassword
	}
	s.mu.Lock()
	s.resetPasswordAttemptState()
	s.mu.Unlock()

	mnemonic := strings.TrimSpace(string(plaintext))
	if !bip39.IsMnemonicValid(mnemonic) {
		return "", fmt.Errorf("%w: corrupted mnemonic", ErrInvalidMnemonic)
	}
	return mnemonic, nil
}

func (s *SeedManager) ChangePassword(oldPassword, newPassword string) error {
	oldPassword = strings.TrimSpace(oldPassword)
	newPassword = strings.TrimSpace(newPassword)
	if oldPassword == "" || newPassword == "" {
		return ErrPasswordRequired
	}

	s.mu.Lock()
	env := s.envelope
	if err := s.ensureUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if env == nil {
		return ErrSeedNotAvailable
	}

	mnemonicBytes, err := DecryptSeed(env, []byte(oldPassword))
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.onFailedPasswordAttempt()
		return ErrInvalidPassword
	}

	newEnv, err := EncryptSeed(mnemonicBytes, []byte(newPassword))
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.envelope = newEnv
	s.resetPasswordAttemptState()
	return nil
}

func (s *SeedManager) ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(strings.TrimSpace(mnemonic))
}

func (s *SeedManager) ensureUnlocked() error {
	if s.lockedUntil.IsZero() {
		return nil
	}
	if s.now().Before(s.lockedUntil) {
		return ErrPasswordLocked
	}
	return nil
}

func (s *SeedManager) onFailedPasswordAttempt() {
	s.failedAttempts++
	backoff := failedAttemptBackoff(s.failedAttempts)
	s.lockedUntil = s.now().Add(backoff)
}

func (s *SeedManager) resetPasswordAttemptState() {
	s.failedAttempts = 0
	s.lockedUntil = time.Time{}
}

func failedAttemptBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	// 1s, 2s, 4s... up to 32s max.
	shift := attempt - 1
	if shift > 5 {
		shift = 5
	}
	return time.Second * time.Duration(1<<shift)
}

func FromKeys(keys *DerivedKeys) (id string, publicKey ed25519.PublicKey, err error) {
	if keys == nil || len(keys.SigningPublicKey) != ed25519.PublicKeySize {
		return "", nil, ErrIdentityInit
	}
	id, err = identitypolicy.BuildIdentityID(keys.SigningPublicKey)
	if err != nil {
		return "", nil, err
	}
	return id, append([]byte(nil), keys.SigningPublicKey...), nil
}
