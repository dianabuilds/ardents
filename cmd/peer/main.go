package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	runtimepkg "github.com/dianabuilds/ardents/internal/core/app/runtime"
	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/transport/quic"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type Status struct {
	State   string   `json:"state"`
	Reasons []string `json:"reasons,omitempty"`
	TSMs    int64    `json:"ts_ms"`
	PeerID  string   `json:"peer_id,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "init":
		initCmd(os.Args[2:])
	case "start":
		startCmd(os.Args[2:])
	case "status":
		statusCmd(os.Args[2:])
	case "service":
		serviceCmd(os.Args[2:])
	case "addressbook":
		addressBookCmd(os.Args[2:])
	case "systemd":
		systemdCmd(os.Args[2:])
	case "install-service":
		installServiceCmd(os.Args[2:])
	case "support":
		supportCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("usage: peer <init|start|status> [flags]")
	fmt.Println("       peer service key <ensure|rotate> [flags]")
	fmt.Println("       peer addressbook <list|add> [flags]")
	fmt.Println("       peer systemd <unit> [flags]")
	fmt.Println("       peer install-service [flags]")
	fmt.Println("       peer support <bundle> [flags]")
	fmt.Println()
	fmt.Println("flags:")
	fmt.Println("  --home <dir>   portable mode (sets ARDENTS_HOME)")
}

func startCmd(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	cfgPath := fs.String("config", "", "path to config file (default: XDG/ARDENTS_HOME)")
	pcap := fs.Bool("pcap", false, "enable packet capture")
	logFormat := fs.String("log.format", "", "log format: json|text (default from config)")
	logFile := fs.String("log.file", "", "write logs to file (relative to run/ if not absolute)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	dirs := mustDirs(*home)
	if *cfgPath == "" {
		*cfgPath = dirs.ConfigPath()
	}
	cfg, err := config.LoadOrInit(*cfgPath)
	if err != nil {
		fatal(err)
	}
	if *pcap {
		cfg.Observability.PcapEnabled = true
	}
	if *logFormat != "" {
		cfg.Observability.LogFormat = *logFormat
	}
	if *logFile != "" {
		cfg.Observability.LogFile = *logFile
	}
	rt := runtimepkg.New(cfg)
	if err := rt.Start(context.Background()); err != nil {
		fatal(err)
	}
	status := Status{State: string(rt.NetState()), Reasons: rt.NetReasons(), TSMs: timeutil.NowUnixMs(), PeerID: rt.PeerID()}
	if err := writeStatus(dirs, status); err != nil {
		fatal(err)
	}
	fmt.Println("peer started")
	fmt.Println("paths:")
	fmt.Println("  config:", dirs.ConfigDir)
	fmt.Println("  data:  ", dirs.DataDir)
	fmt.Println("  run:   ", dirs.RunDir)
	if cfg.Observability.LogFile != "" {
		fmt.Println("  log file:", cfg.Observability.LogFile)
	}
	if addr := rt.QUICAddr(); addr != "" {
		fmt.Println("quic:", addr)
	}
	waitForSignal()
}

func statusCmd(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	cfgPath := fs.String("config", "", "path to config file (default: XDG/ARDENTS_HOME)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	dirs := mustDirs(*home)
	if *cfgPath == "" {
		*cfgPath = dirs.ConfigPath()
	}
	st, err := readStatus(dirs)
	if err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))

	fmt.Println("paths:")
	fmt.Println("  config:", *cfgPath)
	fmt.Println("  data:  ", dirs.DataDir)
	fmt.Println("  run:   ", dirs.RunDir)
	fmt.Println("  addressbook:", dirs.AddressBookPath())
	fmt.Println("  pcap:", dirs.PcapPath())

	cfg, err := config.LoadOrInit(*cfgPath)
	if err != nil {
		fatal(err)
	}
	if cfg.Observability.HealthAddr != "" {
		// #nosec G101 -- health endpoint is loopback-only HTTP by design.
		healthURL := buildHealthURL(cfg.Observability.HealthAddr)
		// #nosec G107 -- URL is derived from trusted config and loopback.
		if h, err := http.Get(healthURL); err == nil {
			defer func() {
				_ = h.Body.Close()
			}()
			body, err := io.ReadAll(h.Body)
			if err != nil {
				fatal(err)
			}
			fmt.Println("healthz:", string(body))
		}
	}
}

func buildHealthURL(addr string) string {
	if addr == "" {
		return ""
	}
	return (&url.URL{Scheme: "http", Host: addr, Path: "/healthz"}).String()
}

func initCmd(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	cfgPath := fs.String("config", "", "path to config file (default: XDG/ARDENTS_HOME)")
	force := fs.Bool("force", false, "overwrite config if exists")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *cfgPath == "" {
		*cfgPath = dirs.ConfigPath()
	}
	if !*force {
		if _, err := os.Stat(*cfgPath); err == nil {
			fmt.Println("config exists:", *cfgPath)
		} else {
			cfg := config.Default()
			if err := config.Save(*cfgPath, cfg); err != nil {
				fatal(err)
			}
			fmt.Println("config created:", *cfgPath)
		}
	} else {
		cfg := config.Default()
		if err := config.Save(*cfgPath, cfg); err != nil {
			fatal(err)
		}
		fmt.Println("config written:", *cfgPath)
	}
	id, err := identity.LoadOrCreate(dirs.IdentityDir())
	if err != nil {
		fatal(err)
	}
	if _, err := quic.LoadOrCreateKeyMaterial(dirs.KeysDir()); err != nil {
		fatal(err)
	}
	if _, err := addressbook.LoadOrInit(dirs.AddressBookPath()); err != nil {
		fatal(err)
	}
	fmt.Println("initialized:")
	fmt.Println("  identity_id:", id.ID)
	fmt.Println("  config dir:  ", dirs.ConfigDir)
	fmt.Println("  data dir:    ", dirs.DataDir)
	fmt.Println("  run dir:     ", dirs.RunDir)
	fmt.Println()
	fmt.Println("next:")
	fmt.Println("  peer start", homeFlagHint(*home))
	fmt.Println("  peer systemd unit --mode=user", homeFlagHint(*home))
}
