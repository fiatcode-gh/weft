package render

import (
	"fmt"
	"os"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

// Theme is the one style definition weft draws markdown with: the read view
// renders through it and the editor colours source lines from it.
type Theme struct {
	// Name is "" for the terminal palette, "notty" under NO_COLOR, else the
	// WEFT_STYLE name.
	Name string
	// Config is what Glamour renders with.
	Config ansi.StyleConfig
	// Formatter is the chroma formatter for code fences: "terminal16" for the
	// terminal palette (it downsamples the Chroma block's hex anchors to the
	// terminal's 16 colours), Glamour's default "terminal256" for a named
	// style so that theme renders at full fidelity.
	Formatter string
	// Link overlays a wiki-link's display text.
	Link lipgloss.Style
	// Markers overlay workflow markers, keyed by marker text. Shared
	// read-only; callers must not mutate it.
	Markers map[string]lipgloss.Style
	// Scheduled and Deadline overlay a whole SCHEDULED:/DEADLINE: stamp.
	Scheduled, Deadline lipgloss.Style
	// Priority overlays a "[#A]" cookie, keyed by its letter. Shared
	// read-only.
	Priority map[string]lipgloss.Style
}

var themeLink = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)

// themeMarkers colour-codes the workflow markers that appear at the start of
// Logseq bullets. Same palette as the todos dashboard so the page view and
// the dashboard read consistently.
var themeMarkers = map[string]lipgloss.Style{
	"TODO":      lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),  // red
	"DOING":     lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true), // yellow
	"LATER":     lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true), // blue
	"WAITING":   lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),  // dim
	"DONE":      lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true), // green
	"CANCELED":  lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true),
	"CANCELLED": lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true),
	"NOW":       lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true), // magenta
}

var (
	themeScheduled = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Italic(true)
	themeDeadline  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Italic(true)
	themePriority  = map[string]lipgloss.Style{
		"A": lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		"B": lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),
		"C": lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	}
)

// CurrentTheme chooses the theme from env vars only — never querying the
// terminal. This is deliberate: Glamour's WithAutoStyle issues OSC 11
// background-colour queries over stdin, which can leave stray reply bytes in
// the terminal's input buffer. When weft is quit and immediately re-opened,
// the next session's termenv reads those stale bytes, fails to parse them,
// and blocks for seconds before timing out. Reading env vars sidesteps the
// problem.
//
// NO_COLOR wins over WEFT_STYLE. An unknown WEFT_STYLE name yields the notty
// theme together with Glamour's own "style not found" error.
func CurrentTheme() (Theme, error) {
	t := Theme{Formatter: "terminal256", Link: themeLink, Markers: themeMarkers,
		Scheduled: themeScheduled, Deadline: themeDeadline, Priority: themePriority}
	name := ""
	if noColor() {
		name = "notty"
	} else if s := os.Getenv("WEFT_STYLE"); s != "" {
		name = s
	} else {
		t.Config = terminalStyleConfig
		t.Formatter = "terminal16"
		return t, nil
	}
	cfg, ok := styles.DefaultStyles[name]
	if !ok {
		t.Name = "notty"
		t.Config = noColorStyleConfig
		return t, fmt.Errorf("%s: style not found", name)
	}
	t.Name = name
	t.Config = *cfg
	if name == "notty" {
		t.Config = noColorStyleConfig
	}
	return t, nil
}
