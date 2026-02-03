package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/dianabuilds/ardents/internal/shared/lockeys"
)

func serviceCmd(args []string) {
	if len(args) < 2 {
		fmt.Println("usage: peer service key <ensure|rotate> [flags]")
		os.Exit(2)
	}
	switch args[0] {
	case "key":
		serviceKeyCmd(args[1:])
	default:
		fmt.Println("usage: peer service key <ensure|rotate> [flags]")
		os.Exit(2)
	}
}

func serviceKeyCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: peer service key <ensure|rotate> [flags]")
		os.Exit(2)
	}
	switch args[0] {
	case "ensure":
		serviceKeyEnsure(args[1:])
	case "rotate":
		serviceKeyRotate(args[1:])
	default:
		fmt.Println("usage: peer service key <ensure|rotate> [flags]")
		os.Exit(2)
	}
}

func serviceKeyEnsure(args []string) {
	serviceKeyAction("ensure", args, func(dir, serviceID string) error {
		_, err := lockeys.LoadOrCreate(dir, serviceID)
		return err
	})
}

func serviceKeyRotate(args []string) {
	serviceKeyAction("rotate", args, func(dir, serviceID string) error {
		_, err := lockeys.Rotate(dir, serviceID)
		return err
	})
}

func serviceKeyAction(action string, args []string, fn func(dir, serviceID string) error) {
	fs := flag.NewFlagSet("service key "+action, flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	serviceID := fs.String("service-id", "", "service_id (required)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if *serviceID == "" {
		fatal(errors.New("ERR_CLI_INVALID_ARGS"))
	}
	dirs := mustDirs(*home)
	if err := fn(dirs.LKeysDir(), *serviceID); err != nil {
		fatal(err)
	}
	fmt.Println("service key "+action+":", *serviceID)
}
