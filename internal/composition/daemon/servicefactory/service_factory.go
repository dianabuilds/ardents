package servicefactory

import (
	"aim-chat/go-backend/internal/bootstrap/wakuconfig"
	"aim-chat/go-backend/internal/composition/daemonservice"
	"aim-chat/go-backend/internal/domains/contracts"
)

// BuildDaemonService composes daemon-ready service from config path and data dir.
func BuildDaemonService(configPath, dataDir string) (contracts.DaemonService, error) {
	return daemonservice.NewServiceForDaemonWithDataDir(wakuconfig.LoadFromPath(configPath), dataDir)
}
