package usecase

import (
	"aim-chat/go-backend/internal/crypto"
	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type BackupExportResult struct {
	Blob         string
	IdentityID   string
	MessageCount int
	SessionCount int
}

type backupIdentityReader interface {
	GetIdentity() models.Identity
	Contacts() []models.Contact
}

type backupMessageSnapshotter interface {
	Snapshot() (map[string]models.Message, map[string]storage.PendingMessage)
}

type backupSessionSnapshotter interface {
	Snapshot() ([]crypto.SessionState, error)
}

func ExportBackup(consentToken, passphrase string, identity backupIdentityReader, messageStore backupMessageSnapshotter, sessionManager backupSessionSnapshotter) (BackupExportResult, error) {
	consentToken = strings.TrimSpace(consentToken)
	passphrase = strings.TrimSpace(passphrase)
	if consentToken != "I_UNDERSTAND_BACKUP_RISK" {
		return BackupExportResult{}, errors.New("backup export requires explicit consent token")
	}
	if passphrase == "" {
		return BackupExportResult{}, errors.New("backup passphrase is required")
	}

	messages, pending := messageStore.Snapshot()
	sessions, err := sessionManager.Snapshot()
	if err != nil {
		return BackupExportResult{}, err
	}
	payload := struct {
		Version    int                               `json:"version"`
		ExportedAt time.Time                         `json:"exported_at"`
		Identity   models.Identity                   `json:"identity"`
		Contacts   []models.Contact                  `json:"contacts"`
		Messages   map[string]models.Message         `json:"messages"`
		Pending    map[string]storage.PendingMessage `json:"pending"`
		Sessions   []crypto.SessionState             `json:"sessions"`
	}{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Identity:   identity.GetIdentity(),
		Contacts:   identity.Contacts(),
		Messages:   messages,
		Pending:    pending,
		Sessions:   sessions,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return BackupExportResult{}, err
	}
	encrypted, err := securestore.Encrypt(passphrase, raw)
	if err != nil {
		return BackupExportResult{}, err
	}
	return BackupExportResult{
		Blob:         base64.StdEncoding.EncodeToString(encrypted),
		IdentityID:   payload.Identity.ID,
		MessageCount: len(messages),
		SessionCount: len(sessions),
	}, nil
}

type accountIdentityAccess interface {
	GetIdentity() models.Identity
	VerifyPassword(password string) error
}

type createAccountIdentity interface {
	CreateIdentity(password string) (models.Identity, string, error)
}

type createIdentityAccess interface {
	CreateIdentity(password string) (models.Identity, string, error)
}

type importIdentityAccess interface {
	ImportIdentity(mnemonic, password string) (models.Identity, error)
}

func CreateAccount(password string, identity createAccountIdentity) (models.Account, error) {
	created, _, err := identity.CreateIdentity(password)
	if err != nil {
		return models.Account{}, err
	}
	return models.Account{
		ID:                created.ID,
		IdentityPublicKey: created.SigningPublicKey,
	}, nil
}

func Login(accountID, password string, identity accountIdentityAccess) error {
	current := identity.GetIdentity()
	if err := identitypolicy.ValidateLoginInput(accountID, password, current.ID); err != nil {
		return err
	}
	return identity.VerifyPassword(password)
}

func CreateIdentity(password string, identity createIdentityAccess, persist func() error) (models.Identity, string, error) {
	created, mnemonic, err := identity.CreateIdentity(strings.TrimSpace(password))
	if err != nil {
		return models.Identity{}, "", err
	}
	if err := persist(); err != nil {
		return models.Identity{}, "", err
	}
	return created, mnemonic, nil
}

func ImportIdentity(mnemonic, password string, identity importIdentityAccess, persist func() error) (models.Identity, error) {
	created, err := identity.ImportIdentity(strings.TrimSpace(mnemonic), strings.TrimSpace(password))
	if err != nil {
		return models.Identity{}, err
	}
	if err := persist(); err != nil {
		return models.Identity{}, err
	}
	return created, nil
}
