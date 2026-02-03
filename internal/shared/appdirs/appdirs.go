package appdirs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const EnvHome = "ARDENTS_HOME"

type Dirs struct {
	Home      string
	ConfigDir string
	DataDir   string
	StateDir  string
	RunDir    string
}

func Resolve(overrideHome string) (Dirs, error) {
	if dirs, ok, err := resolveExplicitHome(overrideHome); ok || err != nil {
		return dirs, err
	}

	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd", "darwin":
		return resolveXDGDirs()
	default:
		return resolveWindowsDirs()
	}
}

func resolveExplicitHome(overrideHome string) (Dirs, bool, error) {
	home := firstNonEmpty(overrideHome, os.Getenv(EnvHome))
	if home == "" {
		return Dirs{}, false, nil
	}
	home = filepath.Clean(home)
	if !filepath.IsAbs(home) {
		wd, err := os.Getwd()
		if err != nil {
			return Dirs{}, true, err
		}
		home = filepath.Join(wd, home)
	}
	return Dirs{
		Home:      home,
		ConfigDir: filepath.Join(home, "config"),
		DataDir:   filepath.Join(home, "data"),
		StateDir:  filepath.Join(home, "run"),
		RunDir:    filepath.Join(home, "run"),
	}, true, nil
}

func resolveXDGDirs() (Dirs, error) {
	userHome, err := os.UserHomeDir()
	if err != nil || userHome == "" {
		return Dirs{}, errors.New("ERR_HOME_UNAVAILABLE")
	}
	cfgHome := firstNonEmpty(os.Getenv("XDG_CONFIG_HOME"), filepath.Join(userHome, ".config"))
	dataHome := firstNonEmpty(os.Getenv("XDG_DATA_HOME"), filepath.Join(userHome, ".local", "share"))
	stateHome := firstNonEmpty(os.Getenv("XDG_STATE_HOME"), filepath.Join(userHome, ".local", "state"))
	cfg := filepath.Join(cfgHome, "ardents")
	data := filepath.Join(dataHome, "ardents")
	state := filepath.Join(stateHome, "ardents")
	return Dirs{
		Home:      "",
		ConfigDir: cfg,
		DataDir:   data,
		StateDir:  state,
		RunDir:    filepath.Join(state, "run"),
	}, nil
}

func resolveWindowsDirs() (Dirs, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Dirs{}, errors.New("ERR_HOME_UNAVAILABLE")
		}
		base = wd
	}
	home := filepath.Join(base, "ardents")
	return Dirs{
		Home:      home,
		ConfigDir: filepath.Join(home, "config"),
		DataDir:   filepath.Join(home, "data"),
		StateDir:  filepath.Join(home, "run"),
		RunDir:    filepath.Join(home, "run"),
	}, nil
}

func (d Dirs) ConfigPath() string {
	return filepath.Join(d.ConfigDir, "node.json")
}

func (d Dirs) AddressBookPath() string {
	return filepath.Join(d.DataDir, "addressbook.json")
}

func (d Dirs) IdentityDir() string {
	return filepath.Join(d.DataDir, "identity")
}

func (d Dirs) KeysDir() string {
	return filepath.Join(d.DataDir, "keys")
}

func (d Dirs) LKeysDir() string {
	return filepath.Join(d.DataDir, "lkeys")
}

func (d Dirs) StatusPath() string {
	return filepath.Join(d.RunDir, "status.json")
}

func (d Dirs) GatewayTokenPath() string {
	return filepath.Join(d.RunDir, "peer.token")
}

func (d Dirs) PcapPath() string {
	return filepath.Join(d.RunDir, "pcap.jsonl")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
