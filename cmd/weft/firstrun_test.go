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
	return runFirstRunAsk(t, input, home, cfgPath, seen, false)
}

func runFirstRunAsk(t *testing.T, input, home, cfgPath string, seen edit.Snapshot, askCrash bool) (string, string, error) {
	t.Helper()
	var out bytes.Buffer
	got, err := firstRun(strings.NewReader(input), &out, cfgPath, home, seen, askCrash)
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

func TestFirstRunEOFAfterExistingFolderWithoutNewline(t *testing.T) {
	home, cfg := frEnv(t)
	existing := t.TempDir()

	_, _, err := runFirstRun(t, existing, home, cfg, edit.Snapshot{})

	if !errors.Is(err, errNoAnswer) {
		t.Fatalf("err = %v, want errNoAnswer", err)
	}
	mustNotExist(t, cfg)
}

func TestFirstRunCannotCreateReasks(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	home, cfg := frEnv(t)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })
	target := filepath.Join(parent, "sub", "g")

	_, out, err := runFirstRun(t, target+"\ny\n", home, cfg, edit.Snapshot{})

	if !errors.Is(err, errNoAnswer) {
		t.Fatalf("err = %v, want errNoAnswer", err)
	}
	if !strings.Contains(out, "Cannot create "+target) {
		t.Errorf("output missing mkdir failure:\n%s", out)
	}
	if n := strings.Count(out, "Notes folder [~/notes]: "); n != 2 {
		t.Errorf("folder prompt asked %d times, want 2:\n%s", n, out)
	}
	mustNotExist(t, cfg)
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
	_, err := pickGraph(false, strings.NewReader("\n\n"), &out, edit.Snapshot{}, true)
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
	_, err := pickGraph(true, strings.NewReader("\n\n"), &out, edit.Snapshot{}, true)
	if err == nil || err.Error() != noGraphError().Error() {
		t.Fatalf("err = %v, want %v", err, noGraphError())
	}
	if out.Len() != 0 {
		t.Errorf("output = %q", out.String())
	}
}

func TestFirstRunCrashDefaultNo(t *testing.T) {
	home, cfg := frEnv(t)
	_, out, err := runFirstRunAsk(t, "\n\n\n", home, cfg, edit.Snapshot{}, true)
	t.Logf("transcript:\n%s", out)
	if err != nil {
		t.Fatal(err)
	}
	if c := readCfg(t, cfg); c != "graph = \"~/notes\"\ncrash_reports = false\n" {
		t.Fatalf("config = %q", c)
	}
	if n := strings.Count(out, "Crash reports hold the panic message, stack trace, weft version, OS, architecture and Go version.\n"); n != 1 {
		t.Errorf("explanation count = %d, output %q", n, out)
	}
	if n := strings.Count(out, "Send crash reports to the weft author? [y/N]: "); n != 1 {
		t.Errorf("question count = %d, output %q", n, out)
	}
	if want := "Saved graph = \"~/notes\" and crash_reports = false to " + cfg; !strings.Contains(out, want) {
		t.Errorf("output lacks %q: %q", want, out)
	}
}

func TestFirstRunCrashYes(t *testing.T) {
	home, cfg := frEnv(t)
	if _, _, err := runFirstRunAsk(t, "\n\ny\n", home, cfg, edit.Snapshot{}, true); err != nil {
		t.Fatal(err)
	}
	if c := readCfg(t, cfg); c != "graph = \"~/notes\"\ncrash_reports = true\n" {
		t.Fatalf("config = %q", c)
	}
}

func TestFirstRunCrashReasks(t *testing.T) {
	home, cfg := frEnv(t)
	_, out, err := runFirstRunAsk(t, "\n\nmaybe\nYES\n", home, cfg, edit.Snapshot{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "Send crash reports to the weft author? [y/N]: "); n != 2 {
		t.Errorf("question count = %d, output %q", n, out)
	}
	if n := strings.Count(out, "Crash reports hold"); n != 1 {
		t.Errorf("explanation count = %d", n)
	}
	if c := readCfg(t, cfg); c != "graph = \"~/notes\"\ncrash_reports = true\n" {
		t.Fatalf("config = %q", c)
	}
}

func TestFirstRunCrashEOF(t *testing.T) {
	home, cfg := frEnv(t)
	_, _, err := runFirstRunAsk(t, "\n\n", home, cfg, edit.Snapshot{}, true)
	if !errors.Is(err, errNoAnswer) {
		t.Fatalf("err = %v, want errNoAnswer", err)
	}
	mustNotExist(t, cfg)
}

func TestFirstRunNoDSNNoQuestion(t *testing.T) {
	home, cfg := frEnv(t)
	_, out, err := runFirstRunAsk(t, "\n\n", home, cfg, edit.Snapshot{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(out), "crash reports") {
		t.Errorf("output mentions crash reports: %q", out)
	}
	if c := readCfg(t, cfg); c != "graph = \"~/notes\"\n" {
		t.Fatalf("config = %q", c)
	}
}

func TestFirstRunCrashConfigChanged(t *testing.T) {
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
	_, _, err = runFirstRunAsk(t, "\n\n\n", home, cfg, seen, true)
	if err == nil || err.Error() != cfg+" changed while weft was asking; graph not saved" {
		t.Fatalf("err = %v", err)
	}
	if c := readCfg(t, cfg); c != outside {
		t.Fatalf("config overwritten: %q", c)
	}
}

func TestFirstRunCrashKeepsExistingKeys(t *testing.T) {
	home, cfg := frEnv(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(filepath.Dir(cfg)))
	seed := "editor = \"vim\"\n"
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
	if _, _, err := runFirstRunAsk(t, "\n\n\n", home, cfg, seen, true); err != nil {
		t.Fatal(err)
	}
	if c := readCfg(t, cfg); c != seed+"graph = \"~/notes\"\ncrash_reports = false\n" {
		t.Fatalf("config = %q", c)
	}
	got, _, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Editor != userconfig.Vim || got.Graph != filepath.Join(home, "notes") || got.CrashReports || !got.CrashReportsSet {
		t.Fatalf("loaded %+v", got)
	}
}

func TestAsksCrashReports(t *testing.T) {
	cases := []struct {
		name   string
		hasDSN bool
		cfg    userconfig.Config
		want   bool
	}{
		{"dsn, unset", true, userconfig.Config{}, true},
		{"no dsn, unset", false, userconfig.Config{}, false},
		{"dsn, set true", true, userconfig.Config{CrashReports: true, CrashReportsSet: true}, false},
		{"dsn, set false", true, userconfig.Config{CrashReportsSet: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := asksCrashReports(tc.hasDSN, tc.cfg); got != tc.want {
				t.Errorf("asksCrashReports = %v, want %v", got, tc.want)
			}
		})
	}
}
