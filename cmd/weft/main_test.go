package main

import (
	"log"
	"os"
	"path/filepath"
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
