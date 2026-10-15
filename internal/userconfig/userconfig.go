// Package userconfig reads the user's own weft settings from
// $XDG_CONFIG_HOME/weft/config.toml (~/.config/weft/config.toml when it is
// unset). The file is optional; weft writes it only to save the first-run
// answers (SaveFirstRun).
package userconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/fiatcode-gh/weft/v3/internal/edit"
)

// Editor is the in-app editor's editing style.
type Editor string

const (
	Modeless Editor = "modeless" // the default
	Vim      Editor = "vim"
)

// Config is the validated user config. Graph is the expanded, cleaned
// absolute graph folder, "" when the key is absent.
type Config struct {
	Editor          Editor
	Graph           string
	CrashReports    bool // crash_reports; false when absent
	CrashReportsSet bool // the key is in the file (first run must not append it again)
}

// Default is the config with no file.
func Default() Config { return Config{Editor: Modeless} }

type rawConfig struct {
	Editor       *string `toml:"editor"`
	Graph        *string `toml:"graph"`
	CrashReports any     `toml:"crash_reports"`
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

// Load reads Path and returns the snapshot it parsed, for a later guarded
// write. No path or a missing file is Default with the zero snapshot; any
// other problem is an error naming the file.
func Load() (Config, edit.Snapshot, error) {
	path := Path()
	if path == "" {
		return Default(), edit.Snapshot{}, nil
	}
	snap, err := edit.ReadSnapshot(path)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err // it names the path already; the prefix below must not repeat it
		}
		return Config{}, edit.Snapshot{}, fmt.Errorf("%s: %s", path, err)
	}
	if !snap.Exists {
		return Default(), edit.Snapshot{}, nil
	}
	cfg, err := parse(snap.Content)
	if err != nil {
		return Config{}, edit.Snapshot{}, fmt.Errorf("%s: %s", path, err)
	}
	return cfg, snap, nil
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
	if raw.Graph != nil {
		home, _ := os.UserHomeDir() // "" when unknown; ExpandGraph reports it only if needed
		if cfg.Graph, err = ExpandGraph(*raw.Graph, home); err != nil {
			return Config{}, err
		}
	}
	if raw.CrashReports != nil {
		b, ok := raw.CrashReports.(bool)
		if !ok {
			return Config{}, fmt.Errorf("crash_reports: %s must be true or false", tomlValue(raw.CrashReports))
		}
		cfg.CrashReports, cfg.CrashReportsSet = b, true
	}
	return cfg, nil
}

func tomlValue(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}

// ExpandGraph turns a configured graph value into a cleaned absolute folder:
// "~/rest" is home/rest, an absolute path is cleaned, anything else (relative,
// empty, bare "~", "~user/...") is an error, as is "~/..." with no known home.
func ExpandGraph(value, home string) (string, error) {
	if rest, ok := strings.CutPrefix(value, "~/"); ok {
		if home == "" {
			return "", fmt.Errorf("graph: %q: home folder unknown", value)
		}
		return filepath.Join(home, rest), nil
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return "", fmt.Errorf("graph: %q must be an absolute path or start with ~/", value)
}

// GraphValue is the text to store for the absolute folder abs: "~/rel" when
// abs is strictly inside home, else abs itself.
func GraphValue(abs, home string) string {
	if home == "" {
		return abs
	}
	rel, err := filepath.Rel(home, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs
	}
	return "~/" + filepath.ToSlash(rel)
}

// SaveFirstRun appends the first-run answers to the config text seen and
// writes it to path: graph = "graph", then crash_reports = true|false when
// crashReports is non-nil. One guarded write; edit.ErrChanged if the file no
// longer matches seen. seen must come from a Load whose config had no graph
// key, and no crash_reports key when crashReports is non-nil; appending at the
// end is then always a top-level key, since the parser rejects tables. A
// symlinked config is written through to its target so the link survives; a
// dangling link is an error and nothing is written.
func SaveFirstRun(path string, seen edit.Snapshot, graph string, crashReports *bool) error {
	var line bytes.Buffer
	if err := toml.NewEncoder(&line).Encode(struct {
		Graph string `toml:"graph"`
	}{graph}); err != nil {
		return err
	}
	text := seen.Content
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	target := path
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		target, err = filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("%s is a broken symlink: %w", path, err)
		}
	}
	if crashReports != nil {
		line.WriteString("crash_reports = " + strconv.FormatBool(*crashReports) + "\n")
	}
	return edit.WriteFileIfUnchanged(target, seen, []byte(text+line.String()))
}
