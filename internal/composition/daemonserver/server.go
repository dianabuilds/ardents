package daemonserver

import (
	"aim-chat/go-backend/internal/adapters/rpc"
	"aim-chat/go-backend/internal/api"
	"aim-chat/go-backend/internal/bootstrap/wakuconfig"
)

// NewRPCServerWithOptions wires daemon service and RPC transport.
func NewRPCServerWithOptions(rpcAddr, configPath, dataDir string) (*rpc.Server, error) {
	svc, err := api.NewServiceForDaemonWithDataDir(wakuconfig.LoadFromPath(configPath), dataDir)
	if err != nil {
		return nil, err
	}
	return rpc.NewServerWithService(rpcAddr, svc), nil
}
