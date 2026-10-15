package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v3/internal/edit"
	"github.com/fiatcode-gh/weft/v3/internal/userconfig"
)

// frEnv sets an isolated HOME and returns it with a not-yet-existing config path.
func frEnv(t *testing.T) (home, cfgPath string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	cfgPath = filepath.Join(t.TempDir(), "cfg", "weft", "config.toml")
	return home, cfgPath
}

func runFirstRun(t *testing.T, input, home, cfgPath string, seen edit.Snapshot) (string, string, error) {
	t.Helper()
	var out bytes.Buffer
	got, err := firstRun(strings.NewReader(input), &out, cfgPath, home, seen)
	return got, out.String(), err
}

func readCfg(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	return string(b)
}

func mustNotExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s should not exist (stat err %v)", p, err)
	}
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestFirstRunDefault(t *testing.T) {
	home, cfg := frEnv(t)
	got, out, err := runFirstRun(t, "\n\n", home, cfg, edit.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "notes")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if es := entries(t, want); len(es) != 0 {
		t.Fatalf("new folder not empty: %v", es)
	}
	if es := entries(t, home); len(es) != 1 || es[0] != "notes" {
		t.Fatalf("home contents = %v", es)
	}
	if c := readCfg(t, cfg); c != "graph = \"~/notes\"\n" {
		t.Fatalf("config = %q", c)
	}
	for _, s := range []string{
		"weft keeps your notes in a folder, the graph.\n",
		"Notes folder [~/notes]: ",
		"Create " + want + "? [Y/n]: ",
		"Saved graph = \"~/notes\" to " + cfg,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q:\n%s", s, out)
		}
	}
}

func TestFirstRunExistingFolder(t *testing.T) {
	home, cfg := frEnv(t)
	vault := filepath.Join(t.TempDir(), "vault")
	if err := os.Mkdir(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	got, out, err := runFirstRun(t, vault+"\n", home, cfg, edit.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if got != vault {
		t.Fatalf("path = %q", got)
	}
	if strings.Contains(out, "Create ") {
		t.Errorf("unexpected create question:\n%s", out)
	}
	if c := readCfg(t, cfg); c != "graph = \""+vault+"\"\n" {
		t.Fatalf("config = %q", c)
	}
	if es := entries(t, home); len(es) != 0 {
		t.Fatalf("home contents = %v", es)
	}
	if es := entries(t, vault); len(es) != 0 {
		t.Fatalf("vault contents = %v", es)
	}
}

func TestFirstRunCreateAccepted(t *testing.T) {
	home, cfg := frEnv(t)
	got, _, err := runFirstRun(t, "~/a/b\ny\n", home, cfg, edit.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "a", "b")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if c := readCfg(t, cfg); c != "graph = \"~/a/b\"\n" {
		t.Fatalf("config = %q", c)
	}
}

func TestFirstRunCreateDeclined(t *testing.T) {
	home, cfg := frEnv(t)
	existing := t.TempDir()
	got, _, err := runFirstRun(t, "~/x\nn\n"+existing+"\n", home, cfg, edit.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if got != existing {
		t.Fatalf("path = %q", got)
	}
	mustNotExist(t, filepath.Join(home, "x"))
	if c := readCfg(t, cfg); c != "graph = \""+existing+"\"\n" {
		t.Fatalf("config = %q", c)
	}
}

func TestFirstRunFileReasks(t *testing.T) {
	home, cfg := frEnv(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, out, err := runFirstRun(t, file+"\n"+dir+"\n", home, cfg, edit.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("path = %q", got)
	}
	if !strings.Contains(out, file+" is a file. Choose a folder.") {
		t.Errorf("output missing file note:\n%s", out)
	}
}

func TestFirstRunTildeUser(t *testing.T) {
	home, cfg := frEnv(t)
	existing := t.TempDir()
	got, out, err := runFirstRun(t, "~bob\n"+existing+"\n", home, cfg, edit.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if got != existing {
		t.Fatalf("path = %q", got)
	}
	if !strings.Contains(out, "Use ~/ or a full path.") {
		t.Errorf("output missing tilde note:\n%s", out)
	}
}

func TestFirstRunRelative(t *testing.T) {
	home, cfg := frEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)
	got, _, err := runFirstRun(t, "rel\ny\n", home, cfg, edit.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	// t.Chdir may report a symlink-resolved cwd; compare via Abs of the same.
	want, _ := filepath.Abs("rel")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if c := readCfg(t, cfg); c != "graph = \""+want+"\"\n" {
		t.Fatalf("config = %q", c)
	}
}

func TestFirstRunEOF(t *testing.T) {
	for _, in := range []string{"", "~/n", "~/n\n"} {
		home, cfg := frEnv(t)
		_, _, err := runFirstRun(t, in, home, cfg, edit.Snapshot{})
		if !errors.Is(err, errNoAnswer) {
			t.Errorf("input %q: err = %v, want errNoAnswer", in, err)
		}
		mustNotExist(t, cfg)
		mustNotExist(t, filepath.Join(home, "n"))
	}
}

func TestFirstRunKeepsConfig(t *testing.T) {
	home, cfg := frEnv(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(filepath.Dir(cfg)))
	seed := "# mine\neditor = \"vim\"\n"
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	_, seen, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runFirstRun(t, "\n\n", home, cfg, seen); err != nil {
		t.Fatal(err)
	}
	if c := readCfg(t, cfg); c != seed+"graph = \"~/notes\"\n" {
		t.Fatalf("config = %q", c)
	}
	got, _, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Editor != userconfig.Vim || got.Graph != filepath.Join(home, "notes") {
		t.Fatalf("loaded %+v", got)
	}
}

func TestFirstRunConfigChanged(t *testing.T) {
	home, cfg := frEnv(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(filepath.Dir(cfg)))
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("editor = \"vim\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, seen, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	outside := "editor = \"modeless\"\n# edited elsewhere\n"
	if err := os.WriteFile(cfg, []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = runFirstRun(t, "\n\n", home, cfg, seen)
	if err == nil || err.Error() != cfg+" changed while weft was asking; graph not saved" {
		t.Fatalf("err = %v", err)
	}
	if c := readCfg(t, cfg); c != outside {
		t.Fatalf("config overwritten: %q", c)
	}
}

func TestPickGraphNotInteractive(t *testing.T) {
	_, cfg := frEnv(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(filepath.Dir(cfg)))
	var out bytes.Buffer
	_, err := pickGraph(false, strings.NewReader("\n\n"), &out, edit.Snapshot{})
	if err == nil || err.Error() != noGraphError().Error() {
		t.Fatalf("err = %v, want %v", err, noGraphError())
	}
	if out.Len() != 0 {
		t.Errorf("output = %q", out.String())
	}
	mustNotExist(t, userconfig.Path())
}

func TestPickGraphNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	var out bytes.Buffer
	_, err := pickGraph(true, strings.NewReader("\n\n"), &out, edit.Snapshot{})
	if err == nil || err.Error() != noGraphError().Error() {
		t.Fatalf("err = %v, want %v", err, noGraphError())
	}
	if out.Len() != 0 {
		t.Errorf("output = %q", out.String())
	}
}
