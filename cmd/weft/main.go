package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/fiatcode-gh/weft/v3/internal/doctor"
	"github.com/fiatcode-gh/weft/v3/internal/edit"
	"github.com/fiatcode-gh/weft/v3/internal/layout"
	"github.com/fiatcode-gh/weft/v3/internal/render"
	"github.com/fiatcode-gh/weft/v3/internal/userconfig"
	"github.com/fiatcode-gh/weft/v3/internal/views"
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

	if _, err := exec.LookPath("rg"); err != nil {
		fmt.Fprintln(os.Stderr, "weft: ripgrep (rg) not found on PATH — install it (https://github.com/BurntSushi/ripgrep) and try again.")
		os.Exit(2)
	}

	if flag.Arg(0) == "doctor" {
		os.Exit(runDoctor(flag.Args()[1:], *graphFlag, os.Getenv("WEFT_GRAPH"), os.Stdout, os.Stderr))
	}

	choice, seen, err := resolveGraph(*graphFlag, os.Getenv("WEFT_GRAPH"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "weft: %v\n", err)
		os.Exit(2)
	}
	if choice.path == "" {
		interactive := term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
		choice, err = pickGraph(interactive, os.Stdin, os.Stdout, seen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "weft: %v\n", err)
			os.Exit(2)
		}
	}
	if err := validateGraphPath(choice); err != nil {
		fmt.Fprintf(os.Stderr, "weft: %v\n", err)
		os.Exit(2)
	}
	graphPath := choice.path

	lay, ok := loadLayout(graphPath, os.Stderr)
	if !ok {
		os.Exit(2)
	}

	cfg, ok := loadUserConfig(os.Stderr)
	if !ok {
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

	app := views.New(graphPath, lay, resolvedVersion())
	app.SetVimEditor(cfg.Editor == userconfig.Vim)

	if _, err := tea.NewProgram(app, programOptions(os.Stdout)...).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "weft: %v\n", err)
		os.Exit(1)
	}
}

// graphChoice is the graph folder and the source it came from, for messages.
type graphChoice struct{ path, source string }

// resolveGraph picks --graph, else $WEFT_GRAPH, else the user config's graph
// key. It reads the user config only when flagVal and envVal are both empty;
// seen is that read's snapshot (zero otherwise). An empty path means no
// source named a graph.
func resolveGraph(flagVal, envVal string) (graphChoice, edit.Snapshot, error) {
	switch {
	case flagVal != "":
		return graphChoice{flagVal, "--graph"}, edit.Snapshot{}, nil
	case envVal != "":
		return graphChoice{envVal, "$WEFT_GRAPH"}, edit.Snapshot{}, nil
	}
	cfg, seen, err := userconfig.Load()
	if err != nil {
		return graphChoice{}, edit.Snapshot{}, err
	}
	if cfg.Graph == "" {
		return graphChoice{}, seen, nil
	}
	return graphChoice{cfg.Graph, userconfig.Path()}, seen, nil
}

// validateGraphPath reports whether the chosen graph is an accessible
// directory, naming where the path came from.
func validateGraphPath(c graphChoice) error {
	info, err := os.Stat(c.path)
	if err != nil {
		return fmt.Errorf("graph path %q from %s is not accessible: %w", c.path, c.source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("graph path %q from %s is not a directory", c.path, c.source)
	}
	return nil
}

// noGraphError reports that no source named a graph folder.
func noGraphError() error {
	cfg := userconfig.Path()
	if cfg == "" {
		cfg = "$XDG_CONFIG_HOME/weft/config.toml"
	}
	return fmt.Errorf("no graph folder: pass --graph PATH, set $WEFT_GRAPH, or add graph = \"PATH\" to %s", cfg)
}

const doctorUsage = "usage: weft doctor [--graph PATH]"

// loadLayout reads the graph's layout config. A config error is printed to
// stderr as "weft: <err>" and reported as false; the caller exits 2.
func loadLayout(graphPath string, stderr io.Writer) (layout.Layout, bool) {
	lay, err := layout.Load(graphPath)
	if err != nil {
		fmt.Fprintf(stderr, "weft: %v\n", err)
		return layout.Layout{}, false
	}
	return lay, true
}

// loadUserConfig reads the user config. An error is printed to stderr as
// "weft: <err>" and reported as false; the caller exits 2.
func loadUserConfig(stderr io.Writer) (userconfig.Config, bool) {
	cfg, _, err := userconfig.Load()
	if err != nil {
		fmt.Fprintf(stderr, "weft: %v\n", err)
		return userconfig.Config{}, false
	}
	return cfg, true
}

// runDoctor executes the headless doctor subcommand and returns its exit
// code: 0 clean, 1 findings, 2 operational error (including a bad layout
// config). doctorArgs are the args after the "doctor" word; globalFlag is the
// global --graph value and envGraph is $WEFT_GRAPH. The graph is doctor's own
// --graph, else the global flag, else the env, else the user config's graph
// key; the user config is read only when none of the first three names a
// graph. Doctor never prompts.
func runDoctor(doctorArgs []string, globalFlag, envGraph string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // usage printing is owned by this function
	graphFlag := fs.String("graph", "", "path to Logseq graph (overrides the global --graph and $WEFT_GRAPH)")
	err := fs.Parse(doctorArgs)
	switch {
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprintln(stdout, doctorUsage)
		return 0
	case err != nil, fs.NArg() > 0:
		fmt.Fprintln(stderr, doctorUsage)
		return 2
	}
	choice := graphChoice{*graphFlag, "--graph"}
	if choice.path == "" {
		choice, _, err = resolveGraph(globalFlag, envGraph)
		if err != nil {
			fmt.Fprintf(stderr, "weft: %v\n", err)
			return 2
		}
		if choice.path == "" {
			fmt.Fprintf(stderr, "weft: %v\n", noGraphError())
			return 2
		}
	}
	if err := validateGraphPath(choice); err != nil {
		fmt.Fprintf(stderr, "weft: %v\n", err)
		return 2
	}
	lay, ok := loadLayout(choice.path, stderr)
	if !ok {
		return 2
	}
	report, err := doctor.Run(choice.path, lay)
	if err != nil {
		fmt.Fprintf(stderr, "weft: doctor: %v\n", err)
		return 2
	}
	report.WriteText(stdout)
	if report.HasFindings() {
		return 1
	}
	return 0
}
