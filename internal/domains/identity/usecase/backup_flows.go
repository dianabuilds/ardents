package usecase

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aim-chat/go-backend/internal/crypto"
	identitydomain "aim-chat/go-backend/internal/domains/identity/domain"
	identityports "aim-chat/go-backend/internal/domains/identity/ports"
	"aim-chat/go-backend/internal/securestore"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
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

func ExportBackup(
	consentToken, password string,
	identity identityports.BackupIdentityReader,
	messageStore identityports.BackupMessageSnapshotter,
	sessionManager identityports.BackupSessionSnapshotter,
) (BackupExportResult, error) {
	consentToken = strings.TrimSpace(consentToken)
	password = strings.TrimSpace(password)
	if !identitydomain.IsBackupConsentTokenValid(consentToken) {
		return BackupExportResult{}, errors.New("backup export requires explicit consent token")
	}
	if password == "" {
		return BackupExportResult{}, errors.New("backup password is required")
	}

	messages, pending := messageStore.Snapshot()
	sessions, err := sessionManager.Snapshot()
	if err != nil {
		return BackupExportResult{}, err
	}
	snapshotter, ok := identity.(identityports.BackupIdentitySnapshotter)
	if !ok {
		return BackupExportResult{}, errors.New("identity manager does not support backup private key snapshot")
	}
	_, signingPrivateKey := snapshotter.SnapshotIdentityKeys()
	if len(signingPrivateKey) == 0 {
		return BackupExportResult{}, errors.New("backup export requires identity private key snapshot")
	}

	payload := backupPayload{
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
	encrypted, err := securestore.Encrypt(password, raw)
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

func RestoreBackup(
	consentToken, password, blob string,
	identity identityports.BackupIdentityRestorer,
	messageStore identityports.BackupMessageRestorer,
	sessionManager identityports.BackupSessionRestorer,
) (BackupRestoreResult, error) {
	consentToken = strings.TrimSpace(consentToken)
	password = strings.TrimSpace(password)
	blob = strings.TrimSpace(blob)
	if !identitydomain.IsBackupConsentTokenValid(consentToken) {
		return BackupRestoreResult{}, errors.New("backup restore requires explicit consent token")
	}
	if password == "" {
		return BackupRestoreResult{}, errors.New("backup password is required")
	}
	if blob == "" {
		return BackupRestoreResult{}, errors.New("backup blob is required")
	}

	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return BackupRestoreResult{}, err
	}
	plain, err := securestore.Decrypt(password, raw)
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
