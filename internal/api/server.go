package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"aim-chat/go-backend/internal/adapters/rpc"
	"aim-chat/go-backend/internal/app/contracts"
	"aim-chat/go-backend/internal/bootstrap/wakuconfig"
)

const defaultRPCAddr = rpc.DefaultRPCAddr

type Server struct {
	service   contracts.DaemonService
	initErr   error
	transport *rpc.Server
}

// NewServer is a legacy constructor; prefer explicit composition via NewServerWithService.
// noinspection GoUnusedExportedFunction
func NewServer() *Server {
	return newServerWithFactory(defaultRPCAddr, func() (contracts.DaemonService, error) {
		return NewServiceForDaemonWithDataDir(wakuconfig.LoadFromPath(""), "")
	})
}

// NewServerWithAddr is a legacy constructor; prefer explicit composition via NewServerWithService.
// noinspection GoUnusedExportedFunction
func NewServerWithAddr(rpcAddr string) *Server {
	return newServerWithFactory(rpcAddr, func() (contracts.DaemonService, error) {
		return NewServiceForDaemonWithDataDir(wakuconfig.LoadFromPath(""), "")
	})
}

// NewServerWithOptions is kept for backward compatibility.
// Legacy constructor: prefer explicit composition (create service first, then call NewServerWithService).
// noinspection GoUnusedExportedFunction
func NewServerWithOptions(rpcAddr, configPath, dataDir string) *Server {
	return newServerWithFactory(rpcAddr, func() (contracts.DaemonService, error) {
		return NewServiceForDaemonWithDataDir(wakuconfig.LoadFromPath(configPath), dataDir)
	})
}

func newServerWithFactory(rpcAddr string, factory func() (contracts.DaemonService, error)) *Server {
	requireRPC := requiresRPCToken()
	rpcToken := strings.TrimSpace(os.Getenv("AIM_RPC_TOKEN"))
	if requireRPC && rpcToken == "" {
		return &Server{initErr: errors.New("AIM_RPC_TOKEN is required unless AIM_REQUIRE_RPC_TOKEN=false or AIM_ENV is test/development/local")}
	}
	svc, err := factory()
	if err != nil {
		return &Server{initErr: err}
	}
	return NewServerWithService(rpcAddr, svc)
}

func NewServerWithService(rpcAddr string, svc contracts.DaemonService) *Server {
	requireRPC := requiresRPCToken()
	rpcToken := strings.TrimSpace(os.Getenv("AIM_RPC_TOKEN"))
	if requireRPC && rpcToken == "" {
		return &Server{service: svc, initErr: errors.New("AIM_RPC_TOKEN is required unless AIM_REQUIRE_RPC_TOKEN=false or AIM_ENV is test/development/local")}
	}
	return &Server{service: svc, transport: rpc.NewServerWithService(rpcAddr, svc)}
}

func (s *Server) ensureTransport() *rpc.Server {
	if s.transport == nil {
		s.transport = rpc.NewServerWithService(defaultRPCAddr, s.service)
	}
	return s.transport
}

func (s *Server) Run(ctx context.Context) error {
	if s.initErr != nil {
		return s.initErr
	}
	return s.ensureTransport().Run(ctx)
}

func (s *Server) HandleRPC(w http.ResponseWriter, r *http.Request) {
	s.ensureTransport().HandleRPC(w, r)
}

func (s *Server) HandleRPCStream(w http.ResponseWriter, r *http.Request) {
	s.ensureTransport().HandleRPCStream(w, r)
}

func requiresRPCToken() bool {
	if v, ok := parseBoolEnv("AIM_REQUIRE_RPC_TOKEN"); ok {
		return v
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AIM_ENV"))) {
	case "test", "testing", "dev", "development", "local":
		return false
	default:
		return true
	}
}

func parseBoolEnv(name string) (bool, bool) {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}
