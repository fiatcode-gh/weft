package userconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v3/internal/edit"
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
	home := t.TempDir()
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
		{name: "graph absolute", body: new("graph = \"/tmp/x/../g\"\n"), want: Config{Editor: Modeless, Graph: "/tmp/g"}},
		{name: "graph tilde", body: new("graph = \"~/notes\"\n"), want: Config{Editor: Modeless, Graph: filepath.Join(home, "notes")}},
		{name: "graph and editor", body: new("editor = \"vim\"\ngraph = \"/tmp/g\"\n"), want: Config{Editor: Vim, Graph: "/tmp/g"}},
		{name: "graph relative", body: new("graph = \"notes\"\n"), errPart: `graph: "notes" must be an absolute path or start with ~/`},
		{name: "graph empty", body: new("graph = \"\"\n"), errPart: `graph: "" must be an absolute path or start with ~/`},
		{name: "graph bare tilde", body: new("graph = \"~\"\n"), errPart: `graph: "~" must be an absolute path or start with ~/`},
		{name: "graph tilde user", body: new("graph = \"~bob/n\"\n"), errPart: `graph: "~bob/n" must be an absolute path or start with ~/`},
		{name: "graph wrong type", body: new("graph = 1\n"), errPart: "graph"},
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
			t.Setenv("HOME", home)
			path := filepath.Join(xdg, "weft", "config.toml")
			var wantSnap edit.Snapshot
			if tc.body != nil {
				writeConfig(t, path, *tc.body)
				wantSnap = edit.Snapshot{Content: *tc.body, Exists: true}
			}
			got, snap, err := Load()
			if tc.errPart == "" {
				if err != nil || got != tc.want {
					t.Fatalf("Load() = %+v, %v; want %+v, nil", got, err, tc.want)
				}
				if snap != wantSnap {
					t.Errorf("snapshot = %+v, want %+v", snap, wantSnap)
				}
				return
			}
			if err == nil {
				t.Fatalf("Load() = %+v, nil; want error containing %q", got, tc.errPart)
			}
			if !strings.Contains(err.Error(), tc.errPart) {
				t.Errorf("err = %q, want %q", err, tc.errPart)
			}
			if n := strings.Count(err.Error(), path); n != 1 {
				t.Errorf("err = %q names the path %d times, want once", err, n)
			}
		})
	}
}

func TestLoadGraphHomeUnknown(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", "")
	writeConfig(t, filepath.Join(xdg, "weft", "config.toml"), "graph = \"~/n\"\n")
	_, _, err := Load()
	if err == nil || !strings.Contains(err.Error(), "home folder unknown") {
		t.Fatalf("Load() error = %v, want one containing %q", err, "home folder unknown")
	}
}

func TestLoadNoPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	got, snap, err := Load()
	if err != nil || got != Default() || snap != (edit.Snapshot{}) {
		t.Fatalf("Load() = %+v, %+v, %v; want Default, zero snapshot, nil", got, snap, err)
	}
}

func TestLoadDirectory(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	path := filepath.Join(xdg, "weft", "config.toml")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load()
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Load() error = %v, want one naming %s", err, path)
	}
	if n := strings.Count(err.Error(), path); n != 1 {
		t.Errorf("error %q names the path %d times, want once", err, n)
	}
}

func TestExpandGraph(t *testing.T) {
	const shape = "must be an absolute path or start with ~/"
	tests := []struct {
		name, value, home, want string
		wantErr                 string
	}{
		{name: "tilde rest", value: "~/a/b", home: "/home/u", want: "/home/u/a/b"},
		{name: "tilde slash", value: "~/", home: "/home/u", want: "/home/u"},
		{name: "absolute cleaned", value: "/a/../b/", home: "/home/u", want: "/b"},
		{name: "relative", value: "a/b", home: "/home/u", wantErr: shape},
		{name: "empty", value: "", home: "/home/u", wantErr: shape},
		{name: "bare tilde", value: "~", home: "/home/u", wantErr: shape},
		{name: "tilde user", value: "~bob/n", home: "/home/u", wantErr: shape},
		{name: "home unknown", value: "~/n", home: "", wantErr: "home folder unknown"},
		{name: "absolute without home", value: "/n", home: "", want: "/n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandGraph(tc.value, tc.home)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ExpandGraph(%q, %q) = %q, %v; want error containing %q", tc.value, tc.home, got, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ExpandGraph(%q, %q) = %q, %v; want %q, nil", tc.value, tc.home, got, err, tc.want)
			}
		})
	}
}

func TestGraphValue(t *testing.T) {
	tests := []struct{ name, abs, home, want string }{
		{"under home", "/home/u/a/b", "/home/u", "~/a/b"},
		{"equal to home", "/home/u", "/home/u", "/home/u"},
		{"sibling of home", "/home/u2/a", "/home/u", "/home/u2/a"},
		{"outside home", "/srv/g", "/home/u", "/srv/g"},
		{"home unknown", "/home/u/a", "", "/home/u/a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := GraphValue(tc.abs, tc.home); got != tc.want {
				t.Errorf("GraphValue(%q, %q) = %q, want %q", tc.abs, tc.home, got, tc.want)
			}
		})
	}
}

func TestSaveGraph(t *testing.T) {
	setup := func(t *testing.T) (path, home string) {
		t.Helper()
		xdg, home := t.TempDir(), t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		t.Setenv("HOME", home)
		return filepath.Join(xdg, "weft", "config.toml"), home
	}
	read := func(t *testing.T, path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	t.Run("keeps content", func(t *testing.T) {
		path, home := setup(t)
		seed := "# my settings\neditor = \"vim\"\n"
		writeConfig(t, path, seed)
		_, snap, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveGraph(path, snap, "~/notes"); err != nil {
			t.Fatal(err)
		}
		if got, want := read(t, path), seed+"graph = \"~/notes\"\n"; got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
		cfg, _, err := Load()
		if err != nil || cfg.Editor != Vim || cfg.Graph != filepath.Join(home, "notes") {
			t.Errorf("Load() = %+v, %v; want Vim and %s", cfg, err, filepath.Join(home, "notes"))
		}
	})

	t.Run("no trailing newline", func(t *testing.T) {
		path, _ := setup(t)
		writeConfig(t, path, "editor = \"vim\"")
		_, snap, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveGraph(path, snap, "/srv/g"); err != nil {
			t.Fatal(err)
		}
		if got, want := read(t, path), "editor = \"vim\"\ngraph = \"/srv/g\"\n"; got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
	})

	t.Run("creates file and folder", func(t *testing.T) {
		path, _ := setup(t)
		if err := SaveGraph(path, edit.Snapshot{}, "/srv/g"); err != nil {
			t.Fatal(err)
		}
		if got, want := read(t, path), "graph = \"/srv/g\"\n"; got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o644 {
			t.Errorf("mode = %v, want 0644", fi.Mode().Perm())
		}
	})

	t.Run("symlinked config keeps the link", func(t *testing.T) {
		path, _ := setup(t)
		target := filepath.Join(t.TempDir(), "dotfiles", "config.toml")
		seed := "# managed\neditor = \"vim\"\n"
		writeConfig(t, target, seed)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		_, snap, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveGraph(path, snap, "~/notes"); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Lstat(path)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("config is no longer a symlink: %v, %v", fi, err)
		}
		if got, want := read(t, target), seed+"graph = \"~/notes\"\n"; got != want {
			t.Errorf("target = %q, want %q", got, want)
		}
	})

	t.Run("dangling symlink errors", func(t *testing.T) {
		path, _ := setup(t)
		missing := filepath.Join(t.TempDir(), "gone.toml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(missing, path); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		err := SaveGraph(path, edit.Snapshot{}, "/srv/g")
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "broken") {
			t.Fatalf("SaveGraph error = %v, want broken-link error naming %s", err, path)
		}
		if got, rerr := os.Readlink(path); rerr != nil || got != missing {
			t.Errorf("link = %q, %v; want unchanged %q", got, rerr, missing)
		}
		if _, serr := os.Stat(missing); !errors.Is(serr, os.ErrNotExist) {
			t.Errorf("%s should not exist (stat err %v)", missing, serr)
		}
	})

	t.Run("refuses changed file", func(t *testing.T) {
		path, _ := setup(t)
		writeConfig(t, path, "editor = \"vim\"\n")
		_, snap, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		outside := "editor = \"modeless\"\n# edited elsewhere\n"
		writeConfig(t, path, outside)
		if err := SaveGraph(path, snap, "/srv/g"); !errors.Is(err, edit.ErrChanged) {
			t.Fatalf("SaveGraph error = %v, want ErrChanged", err)
		}
		if got := read(t, path); got != outside {
			t.Errorf("file = %q, want the outside bytes %q", got, outside)
		}
	})

	t.Run("refuses a file that appeared", func(t *testing.T) {
		path, _ := setup(t)
		writeConfig(t, path, "editor = \"vim\"\n")
		if err := SaveGraph(path, edit.Snapshot{}, "/srv/g"); !errors.Is(err, edit.ErrChanged) {
			t.Fatalf("SaveGraph error = %v, want ErrChanged", err)
		}
		if got := read(t, path); got != "editor = \"vim\"\n" {
			t.Errorf("file = %q, want it untouched", got)
		}
	})

	t.Run("round-trip escaping", func(t *testing.T) {
		path, _ := setup(t)
		value := "/tmp/a \"q\" \\ é"
		if err := SaveGraph(path, edit.Snapshot{}, value); err != nil {
			t.Fatal(err)
		}
		cfg, _, err := Load()
		if err != nil || cfg.Graph != filepath.Clean(value) {
			t.Errorf("Load() = %+v, %v; want Graph %q", cfg, err, filepath.Clean(value))
		}
	})
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
