package privacy

import (
	"aim-chat/go-backend/internal/securestore"
)

func normalizeStoreConfig(path, secret string) (string, string) {
	return securestore.NormalizeStorageConfig(path, secret)
}

func persistEncryptedJSON(path, secret string, state any) error {
	return securestore.WriteEncryptedJSON(path, secret, state)
}
