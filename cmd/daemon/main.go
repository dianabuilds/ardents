package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"aim-chat/go-backend/internal/composition/daemonserver"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	rpcAddr := flag.String("rpc-addr", "127.0.0.1:8787", "JSON-RPC listen address")
	configPath := flag.String("config", "", "Path to config.yaml (optional)")
	dataDir := flag.String("data-dir", "", "Directory for daemon local data (optional)")
	rpcToken := flag.String("rpc-token", "", "RPC token for Authorization/X-AIM-RPC-Token (optional)")
	transport := flag.String("transport", "", "Network transport override: go-waku | mock")
	flag.Parse()
	if *showVersion {
		fmt.Printf("chat-daemon version=%s commit=%s build_date=%s\n", version, commit, buildDate)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if *rpcToken != "" {
		_ = os.Setenv("AIM_RPC_TOKEN", *rpcToken)
	}
	if *transport != "" {
		_ = os.Setenv("AIM_NETWORK_TRANSPORT", *transport)
	}

	srv, err := daemonserver.NewRPCServerWithOptions(*rpcAddr, *configPath, *dataDir)
	if err != nil {
		log.Fatalf("chat-daemon failed to initialize: %v", err)
	}

	log.Println("chat-daemon starting")
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("chat-daemon failed: %v", err)
	}
	log.Println("chat-daemon stopped")
}
