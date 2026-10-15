package userconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPath(t *testing.T) {
	abs := t.TempDir()
	home := t.TempDir()
	tests := []struct {
		name, xdg, home, want string
	}{
		{"absolute XDG", abs, home, filepath.Join(abs, "weft", "config.toml")},
		{"relative XDG ignored", "rel/dir", home, filepath.Join(home, ".config", "weft", "config.toml")},
		{"XDG unset", "", home, filepath.Join(home, ".config", "weft", "config.toml")},
		{"neither", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			t.Setenv("HOME", tc.home)
			if got := Path(); got != tc.want {
				t.Errorf("Path() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		body    *string // nil: no file
		want    Config
		errPart string // "" means no error; the path is always expected too
	}{
		{name: "missing file", want: Default()},
		{name: "empty file", body: new(""), want: Default()},
		{name: "vim", body: new("editor = \"vim\"\n"), want: Config{Editor: Vim}},
		{name: "modeless", body: new("editor = \"modeless\"\n"), want: Config{Editor: Modeless}},
		{name: "unknown key", body: new("bogus = 1\n"), errPart: `unknown key "bogus"`},
		{name: "unknown value", body: new("editor = \"emacs\"\n"), errPart: `editor: "emacs" must be "modeless" or "vim"`},
		{name: "wrong type", body: new("editor = 1\n"), errPart: "editor"},
		{name: "bad toml", body: new("editor = \"vim\n"), errPart: "toml"},
		{name: "table", body: new("[editor]\nx = 1\n"), errPart: "editor"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			xdg := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", xdg)
			path := filepath.Join(xdg, "weft", "config.toml")
			if tc.body != nil {
				writeConfig(t, path, *tc.body)
			}
			got, err := Load()
			if tc.errPart == "" {
				if err != nil || got != tc.want {
					t.Fatalf("Load() = %+v, %v; want %+v, nil", got, err, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("Load() = %+v, nil; want error containing %q", got, tc.errPart)
			}
			if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.errPart) {
				t.Errorf("err = %q, want path %s and %q", err, path, tc.errPart)
			}
		})
	}
}

func TestLoadNoPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	got, err := Load()
	if err != nil || got != Default() {
		t.Fatalf("Load() = %+v, %v; want Default, nil", got, err)
	}
}

func TestLoadDirectory(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	path := filepath.Join(xdg, "weft", "config.toml")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Load() error = %v, want one naming %s", err, path)
	}
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
