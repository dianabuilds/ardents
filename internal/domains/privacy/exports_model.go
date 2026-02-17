//goland:noinspection GoNameStartsWithPackageName
package privacy

import (
	privacymodel "aim-chat/go-backend/internal/domains/privacy/model"
)

type MessagePrivacyMode = privacymodel.MessagePrivacyMode

const (
	MessagePrivacyContactsOnly = privacymodel.MessagePrivacyContactsOnly
	MessagePrivacyRequests     = privacymodel.MessagePrivacyRequests
	MessagePrivacyEveryone     = privacymodel.MessagePrivacyEveryone
	DefaultMessagePrivacyMode  = privacymodel.DefaultMessagePrivacyMode
)

var (
	ErrInvalidMessagePrivacyMode = privacymodel.ErrInvalidMessagePrivacyMode
	ErrInvalidIdentityID         = privacymodel.ErrInvalidIdentityID
)

//goland:noinspection GoNameStartsWithPackageName
type PrivacySettings = privacymodel.PrivacySettings
type Blocklist = privacymodel.Blocklist

func DefaultPrivacySettings() PrivacySettings {
	return privacymodel.DefaultPrivacySettings()
}

func NormalizePrivacySettings(in PrivacySettings) PrivacySettings {
	return privacymodel.NormalizePrivacySettings(in)
}

func ParseMessagePrivacyMode(raw string) (MessagePrivacyMode, error) {
	return privacymodel.ParseMessagePrivacyMode(raw)
}

func NormalizeIdentityID(identityID string) (string, error) {
	return privacymodel.NormalizeIdentityID(identityID)
}

func NewBlocklist(ids []string) (Blocklist, error) {
	return privacymodel.NewBlocklist(ids)
}
