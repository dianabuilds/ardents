package cliutil

import (
	"os"

	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

// ApplyHome sets ARDENTS_HOME for CLI runs when --home is provided.
// This mirrors existing CLI behavior and keeps appdirs resolution consistent.
func ApplyHome(home string) {
	if home == "" {
		return
	}
	_ = os.Setenv(appdirs.EnvHome, home)
}

func ResolveDirs(home string) (appdirs.Dirs, error) {
	ApplyHome(home)
	return appdirs.Resolve(home)
}

func LoadConfig(home string, cfgPath string) (config.Config, error) {
	dirs, err := ResolveDirs(home)
	if err != nil {
		return config.Config{}, err
	}
	if cfgPath == "" {
		cfgPath = dirs.ConfigPath()
	}
	return config.LoadOrInit(cfgPath)
}

func LoadAddressBook(home string) (addressbook.Book, error) {
	dirs, err := ResolveDirs(home)
	if err != nil {
		return addressbook.Book{}, err
	}
	return addressbook.LoadOrInit(dirs.AddressBookPath())
}
