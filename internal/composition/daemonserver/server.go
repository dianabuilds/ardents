package daemonserver

import (
	"aim-chat/go-backend/internal/adapters/rpc"
	"aim-chat/go-backend/internal/composition/daemon/servicefactory"
)

// NewRPCServerWithOptions wires daemon service and RPC transport.
func NewRPCServerWithOptions(rpcAddr, configPath, dataDir string) (*rpc.Server, error) {
	svc, err := servicefactory.BuildDaemonService(configPath, dataDir)
	if err != nil {
		return nil, err
	}
	return rpc.NewServerWithService(rpcAddr, svc), nil
}
