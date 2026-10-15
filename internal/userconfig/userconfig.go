// Package userconfig reads the user's own weft settings from
// $XDG_CONFIG_HOME/weft/config.toml (~/.config/weft/config.toml when it is
// unset). The file is optional and weft never writes it.
package userconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Editor is the in-app editor's editing style.
type Editor string

const (
	Modeless Editor = "modeless" // the default
	Vim      Editor = "vim"
)

// Config is the validated user config.
type Config struct{ Editor Editor }

// Default is the config with no file.
func Default() Config { return Config{Editor: Modeless} }

type rawConfig struct {
	Editor *string `toml:"editor"`
}

// Path is the config file path: $XDG_CONFIG_HOME/weft/config.toml when that
// is an absolute path, else <home>/.config/weft/config.toml; "" when neither
// is known. A relative XDG_CONFIG_HOME is ignored, as the XDG spec says.
func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "weft", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "weft", "config.toml")
}

// Load reads Path. No path or a missing file is Default; any other problem is
// an error naming the file.
func Load() (Config, error) {
	path := Path()
	if path == "" {
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("%s: %s", path, err)
	}
	cfg, err := parse(string(data))
	if err != nil {
		return Config{}, fmt.Errorf("%s: %s", path, err)
	}
	return cfg, nil
}

func parse(text string) (Config, error) {
	var raw rawConfig
	md, err := toml.Decode(text, &raw)
	if err != nil {
		return Config{}, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		return Config{}, fmt.Errorf("unknown key %q", und[0].String())
	}
	cfg := Default()
	if raw.Editor != nil {
		switch ed := Editor(*raw.Editor); ed {
		case Modeless, Vim:
			cfg.Editor = ed
		default:
			return Config{}, fmt.Errorf("editor: %q must be %q or %q", *raw.Editor, Modeless, Vim)
		}
	}
	return cfg, nil
}
