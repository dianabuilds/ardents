package main

import (
	"aim-chat/go-backend/internal/bootstrap/enrollmenttoken"
	"aim-chat/go-backend/internal/bootstrap/wakuconfig"
	"aim-chat/go-backend/internal/nodeagent"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

const (
	exitOK            = 0
	exitInvalidInput  = 10
	exitNetworkFailed = 20
	exitTokenRejected = 30
	exitTrustFailed   = 40
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(exitInvalidInput)
	}

	switch os.Args[1] {
	case "init":
		runInit(os.Args[2:])
	case "enroll":
		runEnroll(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	default:
		printUsage()
		os.Exit(exitInvalidInput)
	}
}

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dataDir := fs.String("data-dir", ".", "node-agent data directory")
	if err := fs.Parse(args); err != nil {
		writeStderrln(err.Error(), exitInvalidInput)
	}

	svc := nodeagent.New(*dataDir)
	state, created, err := svc.Init()
	if err != nil {
		writeStderrln(err.Error(), exitInvalidInput)
		return
	}
	out := map[string]any{
		"created": created,
		"node_id": state.NodeID,
	}
	if err := printJSON(out); err != nil {
		writeStderrln(err.Error(), exitNetworkFailed)
	}
	os.Exit(exitOK)
}

func runEnroll(args []string) {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	dataDir := fs.String("data-dir", ".", "node-agent data directory")
	token := fs.String("token", "", "enrollment token")
	keys := fs.String("issuer-keys", os.Getenv("AIM_ENROLLMENT_ISSUER_KEYS"), "issuer keys map key_id:base64,key_id:base64")
	if err := fs.Parse(args); err != nil {
		writeStderrln(err.Error(), exitInvalidInput)
	}

	if strings.TrimSpace(*token) == "" {
		writeStderrln("token is required", exitInvalidInput)
	}
	parsedKeys, err := enrollmenttoken.ParseIssuerKeys(*keys)
	if err != nil {
		writeStderrln(err.Error(), exitTrustFailed)
	}
	svc := nodeagent.New(*dataDir)
	enrollment, err := svc.Enroll(*token, parsedKeys)
	if err != nil {
		code := exitInvalidInput
		switch {
		case strings.Contains(err.Error(), "issuer"), strings.Contains(err.Error(), "signature"):
			code = exitTrustFailed
		case strings.Contains(err.Error(), "expired"), strings.Contains(err.Error(), "already used"):
			code = exitTokenRejected
		}
		writeStderrln(err.Error(), code)
		return
	}
	if err := printJSON(map[string]any{
		"enrolled":           true,
		"token_id":           enrollment.TokenID,
		"subject_node_group": enrollment.SubjectNodeGroup,
		"expires_at":         enrollment.ExpiresAt,
		"enrolled_at":        enrollment.EnrolledAt,
	}); err != nil {
		writeStderrln(err.Error(), exitNetworkFailed)
	}
	os.Exit(exitOK)
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	dataDir := fs.String("data-dir", ".", "node-agent data directory")
	asJSON := fs.Bool("json", false, "emit json")
	rpcAddr := fs.String("rpc-addr", "", "daemon rpc address host:port")
	rpcToken := fs.String("rpc-token", "", "daemon rpc token")
	if err := fs.Parse(args); err != nil {
		writeStderrln(err.Error(), exitInvalidInput)
	}

	svc := nodeagent.New(*dataDir)
	status, err := svc.Status(context.TODO(), *rpcAddr, *rpcToken)
	if err != nil {
		writeStderrln(err.Error(), exitNetworkFailed)
		return
	}
	if *asJSON {
		if err := printJSON(status); err != nil {
			writeStderrln(err.Error(), exitNetworkFailed)
		}
	} else {
		writeStdoutf(exitNetworkFailed,
			"node_id=%s health=%s peer_count=%d enrolled=%v profile_id=%s\n",
			status.NodeID,
			status.Health,
			status.PeerCount,
			status.Enrolled,
			status.ProfileID,
		)
	}
	os.Exit(exitOK)
}

func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	dataDir := fs.String("data-dir", ".", "node-agent data directory")
	configPath := fs.String("config", "", "daemon config path")
	listenPort := fs.Int("listen-port", 0, "listen port override")
	advertiseAddress := fs.String("advertise-address", "", "advertise address override")
	rpcAddr := fs.String("rpc-addr", "127.0.0.1:8787", "daemon rpc address host:port")
	rpcToken := fs.String("rpc-token", "", "daemon rpc token")
	minPeers := fs.Int("min-peers", 1, "minimum peer count for readiness")
	asJSON := fs.Bool("json", false, "emit json")
	if err := fs.Parse(args); err != nil {
		writeStderrln(err.Error(), exitInvalidInput)
	}

	cfg := wakuconfig.LoadFromPathWithDataDir(*configPath, *dataDir)
	port := *listenPort
	if port <= 0 {
		port = cfg.Port
	}
	adv := strings.TrimSpace(*advertiseAddress)
	if adv == "" {
		adv = cfg.AdvertiseAddress
	}

	svc := nodeagent.New(*dataDir)
	report, err := svc.Doctor(context.TODO(), nodeagent.DoctorInput{
		ListenPort:       port,
		AdvertiseAddress: adv,
		RPCAddr:          *rpcAddr,
		RPCToken:         *rpcToken,
		MinPeers:         *minPeers,
	})
	if err != nil {
		writeStderrln(err.Error(), exitNetworkFailed)
		return
	}
	if *asJSON {
		if err := printJSON(report); err != nil {
			writeStderrln(err.Error(), exitNetworkFailed)
		}
	} else {
		writeStdoutf(exitNetworkFailed, "ready=%v checks=%d\n", report.Ready, len(report.Checks))
		for _, c := range report.Checks {
			if c.Pass {
				writeStdoutf(exitNetworkFailed, "[PASS] %s\n", c.Name)
			} else {
				writeStdoutf(exitNetworkFailed, "[FAIL] %s: %s\n", c.Name, c.Reason)
			}
		}
	}
	if report.Ready {
		os.Exit(exitOK)
	}
	os.Exit(exitNetworkFailed)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printUsage() {
	writeStdoutln(exitInvalidInput, "ardents-node <command> [flags]")
	writeStdoutln(exitInvalidInput, "commands:")
	writeStdoutln(exitInvalidInput, "  init    --data-dir <path>")
	writeStdoutln(exitInvalidInput, "  enroll  --data-dir <path> --token <token> [--issuer-keys key_id:base64,...]")
	writeStdoutln(exitInvalidInput, "  status  --data-dir <path> [--json] [--rpc-addr host:port --rpc-token token]")
	writeStdoutln(exitInvalidInput, "  doctor  --data-dir <path> [--config path] [--listen-port n] [--advertise-address host] [--rpc-addr host:port] [--rpc-token token] [--min-peers n] [--json]")
}

func writeStdoutln(exitCode int, line string) {
	if _, err := fmt.Fprintln(os.Stdout, line); err != nil {
		os.Exit(exitCode)
	}
}

func writeStdoutf(exitCode int, format string, args ...any) {
	if _, err := fmt.Fprintf(os.Stdout, format, args...); err != nil {
		os.Exit(exitCode)
	}
}

func writeStderrln(line string, exitCode int) {
	if _, err := fmt.Fprintln(os.Stderr, line); err != nil {
		os.Exit(exitCode)
	}
	os.Exit(exitCode)
}
