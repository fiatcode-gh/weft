package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("WEFT_TEST_RUN_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
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
