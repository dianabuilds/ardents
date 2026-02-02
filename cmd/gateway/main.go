package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dianabuilds/ardents/internal/addressbook"
	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/gateway"
	"github.com/dianabuilds/ardents/internal/runtime"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

const defaultAddr = "127.0.0.1:8080"

func main() {
	fs := flag.NewFlagSet("gateway", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	addr := fs.String("addr", defaultAddr, "listen address (loopback only)")
	cfgPath := fs.String("config", "", "path to config file (default: XDG/ARDENTS_HOME)")
	tokenPath := fs.String("token", "", "path to peer.token (default: XDG/ARDENTS_HOME)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fatal(err)
	}

	if *home != "" {
		_ = os.Setenv(appdirs.EnvHome, *home)
	}
	dirs, err := appdirs.Resolve(*home)
	if err != nil {
		fatal(err)
	}
	if *cfgPath == "" {
		*cfgPath = dirs.ConfigPath()
	}
	if *tokenPath == "" {
		*tokenPath = dirs.GatewayTokenPath()
	}

	if err := ensureLoopback(*addr); err != nil {
		fatal(err)
	}
	token, err := readToken(*tokenPath)
	if err != nil {
		fatal(err)
	}
	fmt.Println("warn: gateway is loopback-only; token is a secret")

	cfg, err := loadOrInitConfig(*cfgPath)
	if err != nil {
		fatal(err)
	}
	rt := runtime.New(cfg)

	svc := gateway.Service{
		Token: token,
		Send: func(ctx context.Context, addr string, to string, text string) (gateway.AckResult, error) {
			peerID, err := resolveTargetPeerID(dirs, to)
			if err != nil {
				return gateway.AckResult{}, err
			}
			ackBytes, err := rt.SendChat(ctx, addr, peerID, text)
			if err != nil {
				return gateway.AckResult{}, err
			}
			ackEnv, err := envelope.DecodeEnvelope(ackBytes)
			if err != nil {
				return gateway.AckResult{}, err
			}
			p, err := ack.Decode(ackEnv.Payload)
			if err != nil {
				return gateway.AckResult{}, err
			}
			return gateway.AckResult{Status: p.Status, ErrorCode: p.ErrorCode}, nil
		},
		Resolve: func(alias string) (gateway.ResolveResult, error) {
			book, err := addressbook.LoadOrInit(dirs.AddressBookPath())
			if err != nil {
				return gateway.ResolveResult{}, err
			}
			entry, ok, err := book.ResolveAlias(alias, timeutil.NowUnixMs())
			if err != nil {
				return gateway.ResolveResult{}, err
			}
			return gateway.ResolveResult{Found: ok, Entry: entry}, nil
		},
		Status: func() any {
			data, err := os.ReadFile(dirs.StatusPath())
			if err != nil {
				return map[string]any{
					"status": "stopped",
					"ts_ms":  timeutil.NowUnixMs(),
				}
			}
			var out any
			if err := json.Unmarshal(data, &out); err != nil {
				return map[string]any{
					"status": "stopped",
					"ts_ms":  timeutil.NowUnixMs(),
				}
			}
			return out
		},
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           gateway.NewHandler(svc),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

func ensureLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("ERR_GATEWAY_FORBIDDEN_BIND")
	}
	if host == "127.0.0.1" || host == "localhost" || host == "::1" {
		return nil
	}
	return errors.New("ERR_GATEWAY_FORBIDDEN_BIND")
}

func readToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	return token, nil
}

func resolveTargetPeerID(dirs appdirs.Dirs, to string) (string, error) {
	if err := ids.ValidatePeerID(to); err == nil {
		return to, nil
	}
	book, err := addressbook.LoadOrInit(dirs.AddressBookPath())
	if err != nil {
		return "", err
	}
	entry, ok, err := book.ResolveAlias(to, timeutil.NowUnixMs())
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("ERR_ALIAS_NOT_FOUND")
	}
	if entry.TargetType != "peer" {
		return "", errors.New("ERR_ALIAS_TARGET_NOT_PEER")
	}
	return entry.TargetID, nil
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func loadOrInitConfig(path string) (config.Config, error) {
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, nil
	}
	cfg = config.Default()
	if err := config.Save(path, cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}
