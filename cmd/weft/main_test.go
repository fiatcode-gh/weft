package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugLogEnabled(t *testing.T) {
	// arrange
	cases := map[string]bool{
		"":         false,
		"0":        false,
		"false":    false,
		"FALSE":    false,
		"no":       false,
		"off":      false,
		"1":        true,
		"true":     true,
		"yes":      true,
		"weft.log": true,
	}

	for v, want := range cases {
		// act
		got := debugLogEnabled(v)

		// assert
		if got != want {
			t.Errorf("debugLogEnabled(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestInitDebugLogDisabledIsNoOp(t *testing.T) {
	f, err := initDebugLog("")
	if err != nil || f != nil {
		t.Fatalf("disabled flag must be a no-op, got f=%v err=%v", f, err)
	}
}

func TestInitDebugLogOpensFile(t *testing.T) {
	// arrange
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Cleanup(func() { log.SetOutput(os.Stderr) }) // tea.LogToFile redirects the stdlib logger

	// act
	f, err := initDebugLog("1")

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("expected an open log file")
	}
	f.Close()
	if _, err := os.Stat(filepath.Join(cache, "weft", "weft.log")); err != nil {
		t.Fatalf("log file should exist: %v", err)
	}
}

// The user explicitly asked for logging; a swallowed open error means the
// request silently no-ops and the alt-screen hides that anything is wrong.
func TestInitDebugLogFailsWhenLogUnopenable(t *testing.T) {
	// arrange — a directory where the log file should be forces the open error
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	if err := os.MkdirAll(filepath.Join(cache, "weft", "weft.log"), 0o755); err != nil {
		t.Fatal(err)
	}

	// act + assert
	if _, err := initDebugLog("1"); err == nil {
		t.Fatal("expected an error when the debug log cannot be opened")
	}
}

func TestResolveGraphPath(t *testing.T) {
	cases := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{"flag wins over env", "/flag/graph", "/env/graph", "/flag/graph"},
		{"env fallback when flag empty", "", "/env/graph", "/env/graph"},
		{"both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveGraphPath(tc.flag, tc.env); got != tc.want {
				t.Errorf("resolveGraphPath(%q, %q) = %q, want %q",
					tc.flag, tc.env, got, tc.want)
			}
		})
	}
}

func TestResolvedVersion(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	cases := []struct {
		name string
		set  string
		want string
	}{
		{"ldflags-injected value wins", "v9.9.9", "v9.9.9"},
		// Test binaries carry no usable module version ("" or "(devel)"),
		// so the dev sentinel falls straight through.
		{"dev sentinel falls back to dev", "dev", "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Version = tc.set
			if got := resolvedVersion(); got != tc.want {
				t.Errorf("resolvedVersion() with Version=%q = %q, want %q",
					tc.set, got, tc.want)
			}
		})
	}
}

func TestShortenPseudoVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"dev sentinel", "dev", "dev"},
		{"tagged release", "v0.1.3", "v0.1.3"},
		{"prerelease tag", "v0.2.0-rc1", "v0.2.0-rc1"},
		{
			"pseudo no prior tag",
			"v0.0.0-20260525115608-94836dea2daa",
			"v0.0.0+94836de",
		},
		{
			"pseudo after release",
			"v0.1.3-0.20260525115608-94836dea2daa",
			"v0.1.3-0+94836de",
		},
		{
			"pseudo after prerelease",
			"v0.1.3-pre.0.20260525115608-94836dea2daa",
			"v0.1.3-pre.0+94836de",
		},
		{
			"pseudo with dirty suffix",
			"v0.1.3-0.20260525115608-94836dea2daa+dirty",
			"v0.1.3-0+94836de+dirty",
		},
		{
			"pseudo no prior tag with dirty",
			"v0.0.0-20260525115608-94836dea2daa+dirty",
			"v0.0.0+94836de+dirty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortenPseudoVersion(tc.in); got != tc.want {
				t.Errorf("shortenPseudoVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// writeTempGraph materialises a throwaway graph and returns its root.
func writeTempGraph(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunDoctorExitCodes(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	fixture, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	clean := writeTempGraph(t, map[string]string{
		"pages/Solo.md":  "- links to [[Other]]\n",
		"pages/Other.md": "- links to [[Solo]]\n",
	})

	cases := []struct {
		name  string
		args  []string
		graph string
		want  int
	}{
		{"clean graph exits 0", []string{"--graph", clean}, "", 0},
		{"findings exit 1", []string{"--graph", fixture}, "", 1},
		{"missing graph dir exits 2", []string{"--graph", filepath.Join(t.TempDir(), "nope")}, "", 2},
		{"unknown flag exits 2", []string{"--bogus"}, fixture, 2},
		{"positional arg exits 2", []string{"extra"}, fixture, 2},
		{"global graph fallback finds fixture findings", nil, fixture, 1},
		{"doctor flag wins over findings-laden global", []string{"--graph", clean}, fixture, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if got := runDoctor(tc.args, tc.graph, &buf); got != tc.want {
				t.Errorf("runDoctor(%v, %q) = %d, want %d", tc.args, tc.graph, got, tc.want)
			}
		})
	}
}

func TestRunDoctorHelpPrintsUsage(t *testing.T) {
	// arrange + act
	var buf bytes.Buffer
	got := runDoctor([]string{"-h"}, "ignored", &buf)

	// assert
	if got != 0 {
		t.Errorf("-h exit = %d, want 0", got)
	}
	if !strings.Contains(buf.String(), "usage: weft doctor") {
		t.Errorf("usage not printed to stdout, got %q", buf.String())
	}
}
