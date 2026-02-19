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

type BackupRestoreResult struct {
	IdentityID   string
	MessageCount int
	SessionCount int
}

type backupIdentityReader interface {
	GetIdentity() models.Identity
	Contacts() []models.Contact
}

type backupIdentitySnapshotter interface {
	SnapshotIdentityKeys() (publicKey []byte, privateKey []byte)
	SnapshotSeedEnvelopeJSON() []byte
}

type backupIdentityRestorer interface {
	GetIdentity() models.Identity
	RestoreIdentityPrivateKey(privateKey []byte) error
	AddContactByIdentityID(contactID, displayName string) error
}

type backupMessageSnapshotter interface {
	Snapshot() (map[string]models.Message, map[string]storage.PendingMessage)
}

type backupMessageRestorer interface {
	SaveMessage(msg models.Message) error
	AddOrUpdatePending(message models.Message, retryCount int, nextRetry time.Time, lastErr string) error
}

type backupSessionSnapshotter interface {
	Snapshot() ([]crypto.SessionState, error)
}

type backupSessionRestorer interface {
	RestoreSnapshot(states []crypto.SessionState) error
}

type backupPayload struct {
	Version           int                               `json:"version"`
	ExportedAt        time.Time                         `json:"exported_at"`
	Identity          models.Identity                   `json:"identity"`
	SigningPrivateKey []byte                            `json:"signing_private_key"`
	SeedEnvelope      []byte                            `json:"seed_envelope,omitempty"`
	Contacts          []models.Contact                  `json:"contacts"`
	Messages          map[string]models.Message         `json:"messages"`
	Pending           map[string]storage.PendingMessage `json:"pending"`
	Sessions          []crypto.SessionState             `json:"sessions"`
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
	snapshotter, ok := identity.(backupIdentitySnapshotter)
	if !ok {
		return BackupExportResult{}, errors.New("identity manager does not support backup private key snapshot")
	}
	_, signingPrivateKey := snapshotter.SnapshotIdentityKeys()
	if len(signingPrivateKey) == 0 {
		return BackupExportResult{}, errors.New("backup export requires identity private key snapshot")
	}
	payload := struct {
		Version           int                               `json:"version"`
		ExportedAt        time.Time                         `json:"exported_at"`
		Identity          models.Identity                   `json:"identity"`
		SigningPrivateKey []byte                            `json:"signing_private_key"`
		SeedEnvelope      []byte                            `json:"seed_envelope,omitempty"`
		Contacts          []models.Contact                  `json:"contacts"`
		Messages          map[string]models.Message         `json:"messages"`
		Pending           map[string]storage.PendingMessage `json:"pending"`
		Sessions          []crypto.SessionState             `json:"sessions"`
	}{
		Version:           1,
		ExportedAt:        time.Now().UTC(),
		Identity:          identity.GetIdentity(),
		SigningPrivateKey: signingPrivateKey,
		SeedEnvelope:      snapshotter.SnapshotSeedEnvelopeJSON(),
		Contacts:          identity.Contacts(),
		Messages:          messages,
		Pending:           pending,
		Sessions:          sessions,
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

func RestoreBackup(consentToken, passphrase, blob string, identity backupIdentityRestorer, messageStore backupMessageRestorer, sessionManager backupSessionRestorer) (BackupRestoreResult, error) {
	consentToken = strings.TrimSpace(consentToken)
	passphrase = strings.TrimSpace(passphrase)
	blob = strings.TrimSpace(blob)
	if consentToken != "I_UNDERSTAND_BACKUP_RISK" {
		return BackupRestoreResult{}, errors.New("backup restore requires explicit consent token")
	}
	if passphrase == "" {
		return BackupRestoreResult{}, errors.New("backup passphrase is required")
	}
	if blob == "" {
		return BackupRestoreResult{}, errors.New("backup blob is required")
	}

	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return BackupRestoreResult{}, err
	}
	plain, err := securestore.Decrypt(passphrase, raw)
	if err != nil {
		return BackupRestoreResult{}, err
	}
	var payload backupPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return BackupRestoreResult{}, err
	}
	if payload.Version != 1 {
		return BackupRestoreResult{}, errors.New("backup payload version is invalid")
	}
	if len(payload.SigningPrivateKey) == 0 {
		return BackupRestoreResult{}, errors.New("backup payload does not contain identity private key")
	}

	if wiper, ok := messageStore.(interface{ Wipe() error }); ok {
		if err := wiper.Wipe(); err != nil {
			return BackupRestoreResult{}, err
		}
	}
	if wiper, ok := sessionManager.(interface{ Wipe() error }); ok {
		if err := wiper.Wipe(); err != nil {
			return BackupRestoreResult{}, err
		}
	}

	if err := identity.RestoreIdentityPrivateKey(payload.SigningPrivateKey); err != nil {
		return BackupRestoreResult{}, err
	}
	if len(payload.SeedEnvelope) > 0 {
		if seedRestorer, ok := identity.(interface {
			RestoreSeedEnvelopeJSON(raw []byte) error
		}); ok {
			if err := seedRestorer.RestoreSeedEnvelopeJSON(payload.SeedEnvelope); err != nil {
				return BackupRestoreResult{}, err
			}
		}
	}
	if restoredIdentity := identity.GetIdentity(); payload.Identity.ID != "" && restoredIdentity.ID != payload.Identity.ID {
		return BackupRestoreResult{}, errors.New("backup identity id mismatch")
	}

	for _, contact := range payload.Contacts {
		if err := identity.AddContactByIdentityID(contact.ID, contact.DisplayName); err != nil {
			return BackupRestoreResult{}, err
		}
	}
	for _, message := range payload.Messages {
		if err := messageStore.SaveMessage(message); err != nil {
			return BackupRestoreResult{}, err
		}
	}
	for _, pending := range payload.Pending {
		if err := messageStore.AddOrUpdatePending(
			pending.Message,
			pending.RetryCount,
			pending.NextRetry,
			pending.LastError,
		); err != nil {
			return BackupRestoreResult{}, err
		}
	}
	if err := sessionManager.RestoreSnapshot(payload.Sessions); err != nil {
		return BackupRestoreResult{}, err
	}

	return BackupRestoreResult{
		IdentityID:   identity.GetIdentity().ID,
		MessageCount: len(payload.Messages),
		SessionCount: len(payload.Sessions),
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
