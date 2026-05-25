package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode/logseq-tui/internal/views"
)

func main() {
	defaultGraph := os.ExpandEnv("$HOME/Documents/fiat-codex")
	graphFlag := flag.String("graph", "", "path to Logseq graph (overrides $LSTUI_GRAPH and default)")
	flag.Parse()

	graphPath := resolveGraphPath(*graphFlag, os.Getenv("LSTUI_GRAPH"), defaultGraph)

	if _, err := exec.LookPath("rg"); err != nil {
		fmt.Fprintln(os.Stderr, "lstui: ripgrep (rg) not found on PATH — install it (https://github.com/BurntSushi/ripgrep) and try again.")
		os.Exit(2)
	}
	if _, err := os.Stat(graphPath); err != nil {
		fmt.Fprintf(os.Stderr, "lstui: graph path %q is not accessible: %v\n", graphPath, err)
		os.Exit(2)
	}

	app := views.New(graphPath)

	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "lstui: %v\n", err)
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
