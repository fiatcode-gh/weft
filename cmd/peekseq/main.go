package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/peekseq/internal/render"
	"git.fiatcode.dev/fiatcode/peekseq/internal/views"
)

// debugLogPath returns the path Bubble Tea should write debug output to, or
// empty to disable. Enabled by setting PEEKSEQ_DEBUG=1 (writes to ./peekseq.log).
func debugLogPath() string {
	if os.Getenv("PEEKSEQ_DEBUG") != "" {
		return "peekseq.log"
	}
	return ""
}

func main() {
	defaultGraph := os.ExpandEnv("$HOME/Documents/fiat-codex")
	graphFlag := flag.String("graph", "", "path to Logseq graph (overrides $PEEKSEQ_GRAPH and default)")
	flag.Parse()

	graphPath := resolveGraphPath(*graphFlag, os.Getenv("PEEKSEQ_GRAPH"), defaultGraph)

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

	app := views.New(graphPath)

	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "peekseq: %v\n", err)
		os.Exit(1)
	}
}

func resolveGraphPath(flagVal, envVal, defaultVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if envVal != "" {
		return envVal
	}
	return defaultVal
}
