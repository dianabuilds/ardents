package identity

import (
	"aim-chat/go-backend/internal/crypto"
	identitypolicy "aim-chat/go-backend/internal/domains/identity/policy"
	identityusecase "aim-chat/go-backend/internal/domains/identity/usecase"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

func DecodeAttachmentInput(name, mimeType, dataBase64 string) (string, string, []byte, error) {
	return identitypolicy.DecodeAttachmentInput(name, mimeType, dataBase64)
}

func ValidateAttachmentID(attachmentID string) (string, error) {
	return identitypolicy.ValidateAttachmentID(attachmentID)
}

func ExportBackup(
	consentToken, passphrase string,
	identity interface {
		GetIdentity() models.Identity
		Contacts() []models.Contact
	},
	messageStore interface {
		Snapshot() (map[string]models.Message, map[string]storage.PendingMessage)
	},
	sessionManager interface {
		Snapshot() ([]crypto.SessionState, error)
	},
) (BackupExportResult, error) {
	return identityusecase.ExportBackup(consentToken, passphrase, identity, messageStore, sessionManager)
}

func CreateAccount(password string, identity interface {
	CreateIdentity(password string) (models.Identity, string, error)
}) (models.Account, error) {
	return identityusecase.CreateAccount(password, identity)
}

func Login(accountID, password string, identity interface {
	GetIdentity() models.Identity
	VerifyPassword(password string) error
}) error {
	return identityusecase.Login(accountID, password, identity)
}

func CreateIdentity(password string, identity interface {
	CreateIdentity(password string) (models.Identity, string, error)
}, persist func() error) (models.Identity, string, error) {
	return identityusecase.CreateIdentity(password, identity, persist)
}

func ImportIdentity(mnemonic, password string, identity interface {
	ImportIdentity(mnemonic, password string) (models.Identity, error)
}, persist func() error) (models.Identity, error) {
	return identityusecase.ImportIdentity(mnemonic, password, identity, persist)
}
