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
	got := runDoctor([]string{"--graph", dir}, "", &out, &errb)

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
