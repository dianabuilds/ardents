package rpc

import (
	"errors"
)

// Deprecated: construct a daemon service explicitly and use NewServerWithService.
// noinspection GoUnusedExportedFunction
func NewServer() *Server {
	return &Server{initErr: errors.New("NewServer is deprecated; use NewServerWithService")}
}

// Deprecated: construct a daemon service explicitly and use NewServerWithService.
// noinspection GoUnusedExportedFunction
func NewServerWithAddr(rpcAddr string) *Server {
	_ = rpcAddr
	return &Server{initErr: errors.New("NewServerWithAddr is deprecated; use NewServerWithService")}
}

// Deprecated: construct a daemon service explicitly and use NewServerWithService.
// noinspection GoUnusedExportedFunction
func NewServerWithOptions(rpcAddr, configPath, dataDir string) *Server {
	_, _, _ = rpcAddr, configPath, dataDir
	return &Server{initErr: errors.New("NewServerWithOptions is deprecated; use NewServerWithService")}
}
