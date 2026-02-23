package domain

import "strings"

const BackupConsentToken = "I_UNDERSTAND_BACKUP_RISK"

func IsBackupConsentTokenValid(token string) bool {
	return strings.TrimSpace(token) == BackupConsentToken
}
