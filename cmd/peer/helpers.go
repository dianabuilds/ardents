package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

func writeStatus(dirs appdirs.Dirs, st Status) error {
	if err := os.MkdirAll(dirs.RunDir, 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dirs.StatusPath(), b, 0o600)
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
