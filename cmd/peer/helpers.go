package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/perm"
)

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
