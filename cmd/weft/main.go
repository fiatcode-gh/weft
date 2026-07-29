package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/render"
	"git.fiatcode.dev/fiatcode/weft/v2/internal/views"
)

// Version is set at build time via -ldflags "-X main.Version=...". For users
// installing via `go install`, the actual module version is read from
// runtime/debug.BuildInfo and overrides this default.
var Version = "dev"

// debugLogEnabled interprets WEFT_DEBUG: empty and conventional falsy values
// ("0", "false", "no", "off", case-insensitive) disable logging; anything
// else — including unrecognized values — enables it.
func debugLogEnabled(v string) bool {
	switch strings.ToLower(v) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// resolvedVersion returns the version to print for -version. Prefers an
// ldflags-injected value; falls back to the module version captured by `go
// install` / `go build`; falls back to "dev" for raw local builds.
// Pseudo-versions (untagged go-install builds) are shortened so the help
// overlay footer doesn't get cluttered with timestamps.
func resolvedVersion() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return shortenPseudoVersion(v)
		}
	}
	return Version
}

// pseudoVersionRe matches a Go pseudo-version's trailing timestamp + commit
// hash, optionally followed by Go's "+dirty" VCS-modified marker. Form 1
// (no prior tag): "vX.0.0-yyyymmddhhmmss-abcdefabcdef" — separator before
// the timestamp is "-". Form 2/3 (after a release or prerelease tag):
// "...x.yyyymmddhhmmss-abcdefabcdef" — separator is ".".
var pseudoVersionRe = regexp.MustCompile(`^(.+)[-.](\d{14})-([a-f0-9]{12})(\+dirty)?$`)

// shortenPseudoVersion collapses a Go pseudo-version to "<base>+<sha7>",
// dropping the timestamp and truncating the SHA to 7 chars. The "+dirty"
// marker (added by go build against an uncommitted working tree) is
// preserved. Non-pseudo versions (tagged releases, "dev", arbitrary
// strings) are returned unchanged.
func shortenPseudoVersion(v string) string {
	m := pseudoVersionRe.FindStringSubmatch(v)
	if m == nil {
		return v
	}
	return m[1] + "+" + m[3][:7] + m[4]
}

// initDebugLog wires the stdlib logger (Bubble Tea's debug mirror) to the
// weft debug log when the WEFT_DEBUG value enables it. Returns the file to
// close on exit. An open failure is returned, not swallowed: the user
// explicitly asked for logging, and pre-alt-screen is the one moment stderr
// can still tell them it isn't happening.
func initDebugLog(val string) (io.Closer, error) {
	if !debugLogEnabled(val) {
		return nil, nil
	}
	f, err := tea.LogToFile(views.DebugLogPath(), "weft")
	if err != nil {
		return nil, err
	}
	return f, nil
}

func main() {
	graphFlag := flag.String("graph", "", "path to Logseq graph (overrides $WEFT_GRAPH)")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(resolvedVersion())
		return
	}

	graphPath := resolveGraphPath(*graphFlag, os.Getenv("WEFT_GRAPH"))
	if graphPath == "" {
		fmt.Fprintln(os.Stderr, "weft: no graph path — pass --graph or set $WEFT_GRAPH")
		os.Exit(2)
	}

	if _, err := exec.LookPath("rg"); err != nil {
		fmt.Fprintln(os.Stderr, "weft: ripgrep (rg) not found on PATH — install it (https://github.com/BurntSushi/ripgrep) and try again.")
		os.Exit(2)
	}
	if info, err := os.Stat(graphPath); err != nil {
		fmt.Fprintf(os.Stderr, "weft: graph path %q is not accessible: %v\n", graphPath, err)
		os.Exit(2)
	} else if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "weft: graph path %q is not a directory\n", graphPath)
		os.Exit(2)
	}

	debugLog, err := initDebugLog(os.Getenv("WEFT_DEBUG"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "weft: WEFT_DEBUG is set but the debug log can't be opened: %v\n", err)
		os.Exit(2)
	}
	if debugLog != nil {
		defer debugLog.Close()
	}

	// Pre-build the Glamour renderer cache so chroma's syntax-highlighter
	// init cost is paid before the TUI takes over the screen.
	render.Warmup()

	app := views.New(graphPath, resolvedVersion())

	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "weft: %v\n", err)
		os.Exit(1)
	}
}

func resolveGraphPath(flagVal, envVal string) string {
	if flagVal != "" {
		return flagVal
	}
	return envVal
}
