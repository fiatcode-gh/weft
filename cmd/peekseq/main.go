package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/peekseq/internal/render"
	"git.fiatcode.dev/fiatcode/peekseq/internal/views"
)

// Version is set at build time via -ldflags "-X main.Version=...". For users
// installing via `go install`, the actual module version is read from
// runtime/debug.BuildInfo and overrides this default.
var Version = "dev"

// debugLogPath returns the path Bubble Tea should write debug output to, or
// empty to disable. Enabled by setting PEEKSEQ_DEBUG=1 (writes to ./peekseq.log).
func debugLogPath() string {
	if os.Getenv("PEEKSEQ_DEBUG") != "" {
		return "peekseq.log"
	}
	return ""
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

func main() {
	graphFlag := flag.String("graph", "", "path to Logseq graph (overrides $PEEKSEQ_GRAPH)")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(resolvedVersion())
		return
	}

	graphPath := resolveGraphPath(*graphFlag, os.Getenv("PEEKSEQ_GRAPH"))
	if graphPath == "" {
		fmt.Fprintln(os.Stderr, "peekseq: no graph path — pass --graph or set $PEEKSEQ_GRAPH")
		os.Exit(2)
	}

	if _, err := exec.LookPath("rg"); err != nil {
		fmt.Fprintln(os.Stderr, "peekseq: ripgrep (rg) not found on PATH — install it (https://github.com/BurntSushi/ripgrep) and try again.")
		os.Exit(2)
	}
	if _, err := os.Stat(graphPath); err != nil {
		fmt.Fprintf(os.Stderr, "peekseq: graph path %q is not accessible: %v\n", graphPath, err)
		os.Exit(2)
	}

	if path := debugLogPath(); path != "" {
		if f, err := tea.LogToFile(path, "peekseq"); err == nil {
			defer f.Close()
		}
	}

	// Pre-build the Glamour renderer cache so chroma's syntax-highlighter
	// init cost is paid before the TUI takes over the screen.
	render.Warmup()

	app := views.New(graphPath, resolvedVersion())

	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "peekseq: %v\n", err)
		os.Exit(1)
	}
}

func resolveGraphPath(flagVal, envVal string) string {
	if flagVal != "" {
		return flagVal
	}
	return envVal
}
