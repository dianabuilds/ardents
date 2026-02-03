package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/infra/support"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

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
		fatal(errors.New("ERR_CLI_INVALID_ARGS"))
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
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, []byte(unit), 0o600); err != nil {
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
