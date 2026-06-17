package views

import "github.com/charmbracelet/lipgloss"

// Palette. Numeric ANSI colors so the TUI honors the user's terminal theme.
//
// NOTE: internal/render/page.go and the todos marker map (todos.go) duplicate
// the task-marker colors 9/11/12/8. They are intentionally NOT unified across
// the package boundary — see docs/superpowers/specs/2026-05-29-structural-cleanup-design.md
// (a shared theme package for ~4 constants would be premature).
var (
	colorHighlight = lipgloss.Color("12") // blue: links, positions, group headers, selection bg
	colorCursor    = lipgloss.Color("11") // bright yellow: link cursor, search-match emphasis
	colorSelFg     = lipgloss.Color("0")  // black: selected-row foreground
)

// Styles shared verbatim by multiple views.
var (
	styleTitle  = lipgloss.NewStyle().Bold(true)
	styleFaint  = lipgloss.NewStyle().Faint(true)
	styleSel    = lipgloss.NewStyle().Foreground(colorSelFg).Background(colorHighlight).Bold(true)
	styleBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	// styleSyncDirty colours the status-bar "unsynced" dot — yellow for
	// attention, matching the cursor/emphasis accent.
	styleSyncDirty = lipgloss.NewStyle().Foreground(colorCursor)
)
