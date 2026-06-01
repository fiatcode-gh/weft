package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Help is a stateless overlay listing the keymap. ? or esc toggles it off.
type Help struct {
	version string
	width   int
}

// NewHelp builds the help overlay at the current terminal width. width is
// used to clamp rendered lines so the panel never exceeds the terminal —
// pass 0 to disable clamping (content sizes itself; useful in tests that
// don't care about width).
func NewHelp(version string, width int) *Help {
	return &Help{version: version, width: width}
}

// SetSize updates the cached terminal width.
func (h *Help) SetSize(w, _ int) { h.width = w }

// Update reports whether the overlay should close. There's no state to mutate.
func (h *Help) Update(key string) OverlayResult {
	switch key {
	case keyEsc, "?", keyQ:
		return OverlayResult{Cancel: true}
	}
	return OverlayResult{}
}

var (
	helpBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2)
	helpKey     = lipgloss.NewStyle().Bold(true).Foreground(colorHighlight)
	helpSection = lipgloss.NewStyle().Faint(true).Italic(true)
)

type helpRow struct{ key, desc string }

var helpSections = []struct {
	title string
	rows  []helpRow
}{
	{"Browse", []helpRow{
		{"j / k", "scroll one line"},
		{"ctrl-d", "half page down"},
		{"ctrl-u", "half page up"},
		{"g / G", "top / bottom"},
	}},
	{"Links", []helpRow{
		{"n / N", "next / previous wiki-link"},
		{"enter", "follow link under cursor"},
	}},
	{"History", []helpRow{
		{"[", "back"},
		{"]", "forward"},
	}},
	{"Open", []helpRow{
		{".", "today's journal"},
		{"<", "previous journal"},
		{">", "next journal"},
		{"ctrl-p", "picker — find any page"},
		{"/", "search the graph (ripgrep)"},
		{"b", "backlinks to the current page"},
		{"T", "open todos dashboard"},
	}},
	{"Maintain", []helpRow{
		{"R", "rebuild index"},
	}},
	{"Help / quit", []helpRow{
		{"?", "toggle this help"},
		{"esc", "close overlay"},
		{"q", "quit"},
	}},
}

func (h *Help) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Keys"))
	b.WriteString("\n\n")

	const keyCol = 10
	for i, sec := range helpSections {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(helpSection.Render(sec.title))
		b.WriteString("\n")
		for _, r := range sec.rows {
			k := helpKey.Render(r.key)
			pad := keyCol - lipgloss.Width(r.key)
			if pad < 1 {
				pad = 1
			}
			b.WriteString("  ")
			b.WriteString(k)
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(r.desc)
			b.WriteString("\n")
		}
	}

	body := b.String()
	// When the terminal width is known, clamp each line so the panel can't
	// exceed it. The Padding(0, 2) border adds 4 cells of chrome on either
	// side plus the 2 border characters, so the content budget is
	// width - 6. Passing width=0 disables clamping (content sizes itself).
	if h.width > 6 {
		max := h.width - 6
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			lines[i] = clamp(line, max)
		}
		body = strings.Join(lines, "\n")
	}

	// Footer: close hint on the left, version on the right, padded to the
	// max line width of the body built so far. Version segment is omitted
	// when h.version is empty.
	contentWidth := 0
	for _, line := range strings.Split(body, "\n") {
		if w := lipgloss.Width(line); w > contentWidth {
			contentWidth = w
		}
	}

	left := "? or esc to close"
	var footer string
	if h.version == "" {
		footer = left
	} else {
		right := "peekseq " + h.version
		gap := contentWidth - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			// Version too long to fit inline alongside the close hint.
			footer = left + "\n" + right
		} else {
			footer = left + strings.Repeat(" ", gap) + right
		}
	}

	return helpBorder.Render(body + "\n" + styleFaint.Render(footer))
}
