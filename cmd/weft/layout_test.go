package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v3/internal/userconfig"
)

func TestMain(m *testing.M) {
	if os.Getenv("WEFT_TEST_RUN_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	// No test may read the developer's real user config.
	xdg, err := os.MkdirTemp("", "weft-xdg-config")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", xdg)
	code := m.Run()
	os.RemoveAll(xdg)
	os.Exit(code)
}

func writeUserConfig(t *testing.T, body string) string {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	path := filepath.Join(xdg, "weft", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadUserConfig(t *testing.T) {
	t.Run("bad file", func(t *testing.T) {
		path := writeUserConfig(t, "x = 1\n")
		var errb bytes.Buffer

		_, ok := loadUserConfig(&errb)

		want := "weft: " + path + ": unknown key \"x\"\n"
		if ok || errb.String() != want {
			t.Errorf("ok = %v, stderr = %q; want false, %q", ok, errb.String(), want)
		}
	})
	t.Run("valid file", func(t *testing.T) {
		writeUserConfig(t, "editor = \"vim\"\n")
		var errb bytes.Buffer

		cfg, ok := loadUserConfig(&errb)

		if !ok || cfg.Editor != userconfig.Vim || errb.Len() != 0 {
			t.Errorf("= %+v, %v, stderr %q; want vim, true, empty", cfg, ok, errb.String())
		}
	})
}

func TestMainUserConfigErrorExitsTwo(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	dir := writeTempGraph(t, map[string]string{"pages/a.md": "- a\n"})
	path := writeUserConfig(t, "editor = \"emacs\"\n")
	cmd := exec.Command(os.Args[0], "--graph", dir)
	cmd.Env = append(os.Environ(), "WEFT_TEST_RUN_MAIN=1", "XDG_CONFIG_HOME="+filepath.Dir(filepath.Dir(path)))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 2 {
		t.Fatalf("exit = %v, want exit status 2", err)
	}
	want := "weft: " + path + `: editor: "emacs" must be "modeless" or "vim"`
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunDoctorConfigErrorExitsTwo(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	// arrange
	dir := writeTempGraph(t, map[string]string{".weft/config.toml": "bogus = 1\n"})
	var out, errb bytes.Buffer

	// act
	got := runDoctor([]string{"--graph", dir}, "", "", &out, &errb)

	// assert
	if got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	cfg := filepath.Join(dir, ".weft", "config.toml")
	if !strings.Contains(errb.String(), cfg) || !strings.Contains(errb.String(), `unknown key "bogus"`) {
		t.Errorf("stderr = %q, want config path %s and unknown key \"bogus\"", errb.String(), cfg)
	}
}

func TestMainConfigErrorExitsTwo(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	// arrange
	dir := writeTempGraph(t, map[string]string{".weft/config.toml": "namespace_files = \"x\"\n"})
	cmd := exec.Command(os.Args[0], "--graph", dir)
	cmd.Env = append(os.Environ(), "WEFT_TEST_RUN_MAIN=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// act
	err := cmd.Run()

	// assert
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 2 {
		t.Fatalf("exit = %v, want exit status 2", err)
	}
	want := "weft: " + filepath.Join(dir, ".weft", "config.toml") + `: namespace_files: "x" must be "triple-lowbar" or "folders"`
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

// runMain re-execs the test binary as weft with a clean environment: temp HOME
// and XDG_CONFIG_HOME, no inherited WEFT_GRAPH, plus extraEnv. A non-nil
// config body is written to the user config. It returns the exit code, stderr,
// home and config path.
func runMain(t *testing.T, config *string, extraEnv []string, args ...string) (code int, stderr string, home, cfgPath string) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	home = t.TempDir()
	xdg := t.TempDir()
	cfgPath = filepath.Join(xdg, "weft", "config.toml")
	if config != nil {
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfgPath, []byte(*config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "WEFT_GRAPH=") {
			env = append(env, e)
		}
	}
	env = append(env, "WEFT_TEST_RUN_MAIN=1", "HOME="+home, "XDG_CONFIG_HOME="+xdg)
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(env, extraEnv...)
	var errb bytes.Buffer
	cmd.Stderr = &errb

	err := cmd.Run()

	var ee *exec.ExitError
	switch {
	case err == nil:
		code = 0
	case errors.As(err, &ee):
		code = ee.ExitCode()
	default:
		t.Fatal(err)
	}
	return code, errb.String(), home, cfgPath
}

// cleanDoctorGraph is a graph `weft doctor` reports no findings for.
func cleanDoctorGraph(t *testing.T) string {
	t.Helper()
	return writeTempGraph(t, map[string]string{
		"pages/Solo.md":  "- links to [[Other]]\n",
		"pages/Other.md": "- links to [[Solo]]\n",
	})
}

func TestMainDoctorGraphAfterSubcommand(t *testing.T) {
	dir := cleanDoctorGraph(t)
	if code, stderr, _, _ := runMain(t, nil, nil, "doctor", "--graph", dir); code != 0 {
		t.Errorf("exit = %d, want 0 (stderr %q)", code, stderr)
	}
}

func TestMainDoctorGraphBeforeSubcommand(t *testing.T) {
	dir := cleanDoctorGraph(t)
	if code, stderr, _, _ := runMain(t, nil, nil, "--graph", dir, "doctor"); code != 0 {
		t.Errorf("exit = %d, want 0 (stderr %q)", code, stderr)
	}
}

func TestMainDoctorConfigGraph(t *testing.T) {
	dir := cleanDoctorGraph(t)
	cfg := "graph = \"" + dir + "\"\n"
	if code, stderr, _, _ := runMain(t, &cfg, nil, "doctor"); code != 0 {
		t.Errorf("exit = %d, want 0 (stderr %q)", code, stderr)
	}
}

func TestMainNoGraphNotTerminal(t *testing.T) {
	code, stderr, home, cfgPath := runMain(t, nil, nil)

	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if want := "weft: no graph folder: pass --graph PATH, set $WEFT_GRAPH, or add graph = \"PATH\" to " + cfgPath + "\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("config file must not be created, stat err = %v", err)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Errorf("HOME entries = %v (err %v), want none", entries, err)
	}
}

func TestMainGraphSourceErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	cfg := "graph = \"" + missing + "\"\n"
	cases := []struct {
		name   string
		config *string
		env    []string
		args   []string
		source func(cfgPath string) string
	}{
		{"flag", nil, nil, []string{"--graph", missing}, func(string) string { return "--graph" }},
		{"env", nil, []string{"WEFT_GRAPH=" + missing}, nil, func(string) string { return "$WEFT_GRAPH" }},
		{"config", &cfg, nil, nil, func(p string) string { return p }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stderr, _, cfgPath := runMain(t, tc.config, tc.env, tc.args...)

			if code != 2 {
				t.Errorf("exit = %d, want 2", code)
			}
			want := "from " + tc.source(cfgPath) + " is not accessible"
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, want)
			}
		})
	}
}
