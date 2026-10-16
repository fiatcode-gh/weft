package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v3/internal/edit"
	"github.com/fiatcode-gh/weft/v3/internal/userconfig"
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

func TestResolveGraph(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	abs := t.TempDir()

	cases := []struct {
		name       string
		config     string // "" = no config file
		flag, env  string
		wantPath   string
		wantSource string // "" = the config path
		wantErr    bool
		wantSeen   bool // snapshot must equal the config file
		wantCfg    userconfig.Config
	}{
		{name: "flag only", flag: "/f", wantPath: "/f", wantSource: "--graph"},
		{name: "env only", env: "/e", wantPath: "/e", wantSource: "$WEFT_GRAPH"},
		{name: "config only", config: "graph = \"" + abs + "\"\n", wantPath: abs, wantSeen: true},
		{name: "flag beats env and config", config: "graph = \"" + abs + "\"\n", flag: "/f", env: "/e", wantPath: "/f", wantSource: "--graph"},
		{name: "env beats config", config: "graph = \"" + abs + "\"\n", env: "/e", wantPath: "/e", wantSource: "$WEFT_GRAPH"},
		{name: "none", wantPath: ""},
		{name: "config without graph key", config: "editor = \"vim\"\n", wantPath: "", wantSeen: true},
		{name: "malformed config not read when flag set", config: "bogus = 1\n", flag: "/f", wantPath: "/f", wantSource: "--graph"},
		{name: "malformed config not read when env set", config: "bogus = 1\n", env: "/e", wantPath: "/e", wantSource: "$WEFT_GRAPH"},
		{name: "malformed config with nothing set", config: "bogus = 1\n", wantErr: true},
		{name: "tilde expands against HOME", config: "graph = \"~/g\"\n", wantPath: filepath.Join(home, "g"), wantSeen: true},
		{name: "config with crash_reports, no graph", config: "crash_reports = true\n", wantPath: "", wantSeen: true, wantCfg: userconfig.Config{CrashReports: true, CrashReportsSet: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfgPath string
			if tc.config != "" {
				cfgPath = writeUserConfig(t, tc.config)
			} else {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				cfgPath = userconfig.Path()
			}

			got, ucfg, seen, err := resolveGraph(tc.flag, tc.env)

			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), cfgPath) {
					t.Fatalf("err = %v, want error naming %s", err, cfgPath)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.path != tc.wantPath {
				t.Errorf("path = %q, want %q", got.path, tc.wantPath)
			}
			wantSource := tc.wantSource
			if wantSource == "" && tc.wantPath != "" {
				wantSource = cfgPath
			}
			if got.source != wantSource {
				t.Errorf("source = %q, want %q", got.source, wantSource)
			}
			var wantSnap edit.Snapshot
			if tc.wantSeen {
				wantSnap = edit.Snapshot{Content: tc.config, Exists: true}
			}
			if seen != wantSnap {
				t.Errorf("snapshot = %+v, want %+v", seen, wantSnap)
			}
			if ucfg.CrashReports != tc.wantCfg.CrashReports || ucfg.CrashReportsSet != tc.wantCfg.CrashReportsSet {
				t.Errorf("crash config = %v/%v, want %v/%v", ucfg.CrashReports, ucfg.CrashReportsSet, tc.wantCfg.CrashReports, tc.wantCfg.CrashReportsSet)
			}
		})
	}
}

func TestValidateGraphPathNamesSource(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope")
	_, statErr := os.Stat(missing)

	for _, source := range []string{"--graph", "$WEFT_GRAPH", "/cfg/weft/config.toml"} {
		t.Run(source, func(t *testing.T) {
			err := validateGraphPath(graphChoice{missing, source})
			want := fmt.Sprintf("graph path %q from %s is not accessible: %v", missing, source, statErr)
			if err == nil || err.Error() != want {
				t.Errorf("missing: err = %v, want %q", err, want)
			}
			err = validateGraphPath(graphChoice{file, source})
			want = fmt.Sprintf("graph path %q from %s is not a directory", file, source)
			if err == nil || err.Error() != want {
				t.Errorf("file: err = %v, want %q", err, want)
			}
			if err := validateGraphPath(graphChoice{dir, source}); err != nil {
				t.Errorf("directory: err = %v, want nil", err)
			}
		})
	}
}

func TestNoGraphError(t *testing.T) {
	t.Run("with config path", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		msg := noGraphError().Error()
		for _, part := range []string{"--graph", "$WEFT_GRAPH", userconfig.Path()} {
			if !strings.Contains(msg, part) {
				t.Errorf("%q does not name %q", msg, part)
			}
		}
	})
	t.Run("no config path", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		want := `no graph folder: pass --graph PATH, set $WEFT_GRAPH, or add graph = "PATH" to $XDG_CONFIG_HOME/weft/config.toml`
		if got := noGraphError().Error(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
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

	missingDir := filepath.Join(t.TempDir(), "nope")
	cases := []struct {
		name        string
		args        []string
		global      string
		env         string
		config      string // user config body; FIXTURE and MISSING are substituted
		want        int
		wantNoGraph bool // stderr must be the no-graph message
	}{
		{name: "clean graph exits 0", args: []string{"--graph", clean}, want: 0},
		{name: "findings exit 1", args: []string{"--graph", fixture}, want: 1},
		{name: "missing graph dir exits 2", args: []string{"--graph", missingDir}, want: 2},
		{name: "unknown flag exits 2", args: []string{"--bogus"}, global: fixture, want: 2},
		{name: "positional arg exits 2", args: []string{"extra"}, global: fixture, want: 2},
		{name: "global graph fallback finds fixture findings", global: fixture, want: 1},
		{name: "doctor flag wins over findings-laden global", args: []string{"--graph", clean}, global: fixture, want: 0},
		{name: "env graph fallback", env: fixture, want: 1},
		{name: "config graph", config: "graph = \"FIXTURE\"\n", want: 1},
		{name: "no graph anywhere", want: 2, wantNoGraph: true},
		{name: "malformed user config, no graph", config: "bogus = 1\n", want: 2},
		{name: "malformed user config, doctor --graph clean", args: []string{"--graph", clean}, config: "bogus = 1\n", want: 0},
		{name: "config graph missing", config: "graph = \"MISSING\"\n", want: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var cfgPath string
			if tc.config != "" {
				body := strings.NewReplacer("FIXTURE", fixture, "MISSING", missingDir).Replace(tc.config)
				cfgPath = writeUserConfig(t, body)
			} else {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				cfgPath = userconfig.Path()
			}
			var out, errb bytes.Buffer

			got := runDoctor(tc.args, tc.global, tc.env, &out, &errb)

			if got != tc.want {
				t.Errorf("runDoctor(%v, %q, %q) = %d, want %d (stderr %q)", tc.args, tc.global, tc.env, got, tc.want, errb.String())
			}
			if tc.wantNoGraph {
				if want := "weft: " + noGraphError().Error() + "\n"; errb.String() != want {
					t.Errorf("stderr = %q, want %q", errb.String(), want)
				}
			}
			switch tc.name {
			case "malformed user config, no graph":
				if !strings.Contains(errb.String(), cfgPath) {
					t.Errorf("stderr = %q, want it to name %s", errb.String(), cfgPath)
				}
			case "config graph missing":
				if !strings.Contains(errb.String(), "from "+cfgPath+" is not accessible") {
					t.Errorf("stderr = %q, want source %s", errb.String(), cfgPath)
				}
			}
		})
	}
}

func TestRunDoctorHelpPrintsUsage(t *testing.T) {
	// arrange + act
	var buf bytes.Buffer
	got := runDoctor([]string{"-h"}, "ignored", "", &buf, io.Discard)

	// assert
	if got != 0 {
		t.Errorf("-h exit = %d, want 0", got)
	}
	if !strings.Contains(buf.String(), "usage: weft doctor") {
		t.Errorf("usage not printed to stdout, got %q", buf.String())
	}
}
