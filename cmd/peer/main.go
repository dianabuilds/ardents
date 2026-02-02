package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/dianabuilds/ardents/internal/addressbook"
	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/contentnode"
	runtimepkg "github.com/dianabuilds/ardents/internal/runtime"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/perm"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/support"
	"github.com/dianabuilds/ardents/internal/transport/quic"
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
	case "send":
		sendCmd(os.Args[2:])
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
	fmt.Println("usage: peer <init|start|status|send> [flags]")
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
	enableGatewayToken := fs.Bool("enable-gateway", false, "rotate gateway token for cmd/gateway (does not start gateway)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	dirs := mustDirs(*home)
	if *cfgPath == "" {
		*cfgPath = dirs.ConfigPath()
	}
	cfg, err := loadOrInitConfig(*cfgPath)
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

	if *enableGatewayToken {
		if err := rotateGatewayToken(dirs.GatewayTokenPath()); err != nil {
			fatal(err)
		}
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
	if *enableGatewayToken {
		fmt.Println("gateway token:", dirs.GatewayTokenPath())
		fmt.Println("gateway (loopback only): go run ./cmd/gateway", homeFlagHint(*home))
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
	fmt.Println("  gateway token:", dirs.GatewayTokenPath())
	fmt.Println("  pcap:", dirs.PcapPath())

	cfg, err := loadOrInitConfig(*cfgPath)
	if err != nil {
		fatal(err)
	}
	if cfg.Observability.HealthAddr != "" {
		url := "http://" + cfg.Observability.HealthAddr + "/healthz"
		if h, err := http.Get(url); err == nil {
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

func sendCmd(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	to := fs.String("to", "", "alias|peer_id|service_id")
	text := fs.String("text", "", "message text")
	addr := fs.String("addr", "", "quic://host:port (required for now)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	if *to == "" || *text == "" || *addr == "" {
		fatal(errors.New("missing --to or --text or --addr"))
	}

	dirs := mustDirs(*home)
	cfg, err := loadOrInitConfig(dirs.ConfigPath())
	if err != nil {
		fatal(err)
	}
	rt := runtimepkg.New(cfg)
	peerID, err := resolveTargetPeerID(dirs, *to)
	if err != nil {
		fatal(err)
	}
	printIndicators(rt, peerID)
	ackBytes, err := rt.SendChat(context.Background(), *addr, peerID, *text)
	if err != nil {
		fatal(err)
	}
	ackEnv, err := envelope.DecodeEnvelope(ackBytes)
	if err != nil {
		fatal(err)
	}
	ackPayload, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("ack: status=%s error=%s\n", ackPayload.Status, ackPayload.ErrorCode)
	if ackPayload.Status == "REJECTED" {
		fmt.Println("error:", ackPayload.ErrorCode)
	}
}

func resolveTargetPeerID(dirs appdirs.Dirs, to string) (string, error) {
	if to == "" {
		return "", errors.New("missing --to")
	}
	if err := ids.ValidatePeerID(to); err == nil {
		return to, nil
	}
	book, err := addressbook.LoadOrInit(dirs.AddressBookPath())
	if err != nil {
		return "", err
	}
	entry, ok, err := book.ResolveAlias(to, timeutil.NowUnixMs())
	if err != nil {
		if errors.Is(err, addressbook.ErrAliasConflict) {
			return "", addressbook.ErrAliasConflict
		}
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

func addressBookCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: peer addressbook <list|add|export|import> [flags]")
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		addressBookList(args[1:])
	case "add":
		addressBookAdd(args[1:])
	case "export":
		addressBookExport(args[1:])
	case "import":
		addressBookImport(args[1:])
	default:
		fmt.Println("usage: peer addressbook <list|add|export|import> [flags]")
		os.Exit(2)
	}
}

func printIndicators(rt *runtimepkg.Runtime, toID string) {
	trusted := rt.IdentityID() != "" && rt.IdentityID() == toID
	if trusted {
		fmt.Println("trust: trusted")
	} else {
		fmt.Println("trust: untrusted")
	}
	fmt.Println("pow: required for untrusted")
	fmt.Println("net:", rt.NetState())
}

func addressBookList(args []string) {
	fs := flag.NewFlagSet("addressbook list", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	path := fs.String("path", "", "path to addressbook.json (default: XDG/ARDENTS_HOME)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *path == "" {
		*path = dirs.AddressBookPath()
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
}

func addressBookAdd(args []string) {
	fs := flag.NewFlagSet("addressbook add", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	path := fs.String("path", "", "path to addressbook.json (default: XDG/ARDENTS_HOME)")
	alias := fs.String("alias", "", "alias (required)")
	identityID := fs.String("identity", "", "identity_id (did:key:...) required")
	trust := fs.String("trust", "trusted", "trusted|untrusted")
	note := fs.String("note", "", "optional note")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *path == "" {
		*path = dirs.AddressBookPath()
	}

	if *alias == "" || *identityID == "" {
		fatal(errors.New("missing --alias or --identity"))
	}
	if err := ids.ValidateAlias(*alias); err != nil {
		fatal(err)
	}
	if err := ids.ValidateIdentityID(*identityID); err != nil {
		fatal(err)
	}
	if *trust != "trusted" && *trust != "untrusted" {
		fatal(errors.New("trust must be trusted|untrusted"))
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	entry := addressbook.Entry{
		Alias:       *alias,
		TargetType:  "identity",
		TargetID:    *identityID,
		Source:      "self",
		Trust:       *trust,
		Note:        *note,
		CreatedAtMs: timeutil.NowUnixMs(),
	}
	book.Entries = append(book.Entries, entry)
	book.UpdatedAtMs = timeutil.NowUnixMs()
	if err := addressbook.Save(*path, book); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook entry added")
}

func addressBookExport(args []string) {
	fs := flag.NewFlagSet("addressbook export", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	path := fs.String("path", "", "path to addressbook.json (default: XDG/ARDENTS_HOME)")
	out := fs.String("out", "addressbook.bundle.cbor", "output file")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *path == "" {
		*path = dirs.AddressBookPath()
	}

	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	id, err := identity.LoadOrCreate(dirs.IdentityDir())
	if err != nil {
		fatal(err)
	}
	node, err := book.ExportBundle(id)
	if err != nil {
		fatal(err)
	}
	data, err := contentnode.Encode(node)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook bundle exported:", *out)
}

func addressBookImport(args []string) {
	fs := flag.NewFlagSet("addressbook import", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	path := fs.String("path", "", "path to addressbook.json (default: XDG/ARDENTS_HOME)")
	in := fs.String("in", "", "input bundle file (required)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *path == "" {
		*path = dirs.AddressBookPath()
	}

	if *in == "" {
		fatal(errors.New("missing --in"))
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	var node contentnode.Node
	if err := contentnode.Decode(data, &node); err != nil {
		fatal(err)
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	book, err = book.ImportBundle(node, timeutil.NowUnixMs())
	if err != nil {
		fatal(err)
	}
	if err := addressbook.Save(*path, book); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook bundle imported")
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

func writeStatus(dirs appdirs.Dirs, st Status) error {
	if err := os.MkdirAll(dirs.RunDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dirs.StatusPath(), b, 0o644)
}

func readStatus(dirs appdirs.Dirs) (Status, error) {
	b, err := os.ReadFile(dirs.StatusPath())
	if err != nil {
		return Status{}, err
	}
	var st Status
	if err := json.Unmarshal(b, &st); err != nil {
		return Status{}, err
	}
	return st, nil
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}

func rotateGatewayToken(path string) error {
	f, err := perm.OpenOwnerOnly(path)
	if err != nil {
		return errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	defer func() {
		_ = f.Close()
	}()
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := base64.StdEncoding.EncodeToString(raw)
	if _, err := f.WriteString(token); err != nil {
		return err
	}
	return nil
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

func systemdCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: peer systemd unit [--mode user|system] [--home <dir>] [--exec <path>]")
		os.Exit(2)
	}
	switch args[0] {
	case "unit":
		systemdUnit(args[1:])
	default:
		fmt.Println("usage: peer systemd unit [flags]")
		os.Exit(2)
	}
}

func supportCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: peer support bundle [flags]")
		os.Exit(2)
	}
	switch args[0] {
	case "bundle":
		supportBundleCmd(args[1:])
	default:
		fmt.Println("usage: peer support bundle [flags]")
		os.Exit(2)
	}
}

func supportBundleCmd(args []string) {
	fs := flag.NewFlagSet("support bundle", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	out := fs.String("out", "", "output path (default: ./ardents-support-<ts>.zip)")
	lines := fs.Int("lines", 2000, "tail N lines from log file (if enabled)")
	includeBook := fs.Bool("include-addressbook", false, "include full addressbook.json (note field redacted)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	dirs := mustDirs(*home)
	if *out == "" {
		*out = "ardents-support-" + fmt.Sprint(timeutil.NowUnixMs())
	}
	cfg, _ := config.Load(dirs.ConfigPath())

	logPath := ""
	if cfg.Observability.LogFile != "" {
		logPath = cfg.Observability.LogFile
		if !filepath.IsAbs(logPath) {
			logPath = filepath.Join(dirs.RunDir, logPath)
		}
	}
	outPath, err := support.WriteBundle(support.BundleOptions{
		OutPath:            *out,
		ConfigPath:         dirs.ConfigPath(),
		StatusPath:         dirs.StatusPath(),
		AddressBookPath:    dirs.AddressBookPath(),
		LogFilePath:        logPath,
		PcapPath:           dirs.PcapPath(),
		TailLines:          *lines,
		IncludeAddressBook: *includeBook,
	})
	if err != nil {
		fatal(err)
	}
	fmt.Println("support bundle written:", outPath)
}

func systemdUnit(args []string) {
	fs := flag.NewFlagSet("systemd unit", flag.ExitOnError)
	mode := fs.String("mode", "user", "user|system")
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	execPath := fs.String("exec", "", "path to peer executable")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if *execPath == "" {
		if p, err := os.Executable(); err == nil {
			*execPath = p
		} else {
			*execPath = "peer"
		}
	}
	unit := buildSystemdUnit(*mode, *execPath, *home)
	fmt.Print(unit)
}

func buildSystemdUnit(mode string, execPath string, home string) string {
	if mode != "user" && mode != "system" {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=ardents peer\n")
	b.WriteString("After=network-online.target\n")
	b.WriteString("Wants=network-online.target\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	if mode == "system" {
		b.WriteString("User=ardents\n")
		b.WriteString("Group=ardents\n")
	}
	if home != "" {
		b.WriteString("Environment=" + appdirs.EnvHome + "=" + home + "\n")
	}
	b.WriteString("ExecStart=" + execPath + " start\n")
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=2\n")
	b.WriteString("NoNewPrivileges=true\n\n")
	b.WriteString("[Install]\n")
	if mode == "system" {
		b.WriteString("WantedBy=multi-user.target\n")
	} else {
		b.WriteString("WantedBy=default.target\n")
	}
	return b.String()
}

func mustDirs(home string) appdirs.Dirs {
	if home != "" {
		_ = os.Setenv(appdirs.EnvHome, home)
	}
	dirs, err := appdirs.Resolve(home)
	if err != nil {
		fatal(err)
	}
	return dirs
}

func homeFlagHint(home string) string {
	if home == "" {
		return ""
	}
	return "--home " + home
}

func installServiceCmd(args []string) {
	fs := flag.NewFlagSet("install-service", flag.ExitOnError)
	mode := fs.String("mode", "user", "user|system")
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	name := fs.String("name", "ardents", "service name (unit file name without .service)")
	execPath := fs.String("exec", "", "path to peer executable")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if *name == "" {
		fatal(errors.New("missing --name"))
	}
	if *execPath == "" {
		if p, err := os.Executable(); err == nil {
			*execPath = p
		} else {
			*execPath = "peer"
		}
	}
	if *home != "" {
		_ = os.Setenv(appdirs.EnvHome, *home)
	}
	unit := buildSystemdUnit(*mode, *execPath, *home)
	if unit == "" {
		fatal(errors.New("ERR_SYSTEMD_UNIT_INVALID"))
	}
	path, err := systemdUnitPath(*mode, *name)
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		fatal(err)
	}
	fmt.Println("installed:", path)
	fmt.Println("next:")
	if *mode == "system" {
		fmt.Println("  sudo systemctl daemon-reload")
		fmt.Println("  sudo systemctl enable --now", *name+".service")
	} else {
		fmt.Println("  systemctl --user daemon-reload")
		fmt.Println("  systemctl --user enable --now", *name+".service")
	}
}

func systemdUnitPath(mode string, name string) (string, error) {
	filename := name + ".service"
	if mode == "system" {
		return filepath.Join(string(os.PathSeparator), "etc", "systemd", "system", filename), nil
	}
	if mode != "user" {
		return "", errors.New("ERR_SYSTEMD_MODE_INVALID")
	}
	userHome, err := os.UserHomeDir()
	if err != nil || userHome == "" {
		return "", errors.New("ERR_HOME_UNAVAILABLE")
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	base := xdg
	if base == "" {
		base = filepath.Join(userHome, ".config")
	}
	return filepath.Join(base, "systemd", "user", filename), nil
}
