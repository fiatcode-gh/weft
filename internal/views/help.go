package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Help is a stateless overlay listing the keymap. ? or esc toggles it off.
type Help struct {
	version string
}

func NewHelp(version string) *Help { return &Help{version: version} }

// Update reports whether the overlay should close. There's no state to mutate.
func (h *Help) Update(key string) (cancel bool) {
	switch key {
	case "esc", "?", "q":
		return true
	}
	return false
}

var (
	helpBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2)
	helpTitle   = lipgloss.NewStyle().Bold(true)
	helpKey     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	helpFaint   = lipgloss.NewStyle().Faint(true)
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
	b.WriteString(helpTitle.Render("Keys"))
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
	// Footer: close hint on the left, version on the right, padded to the
	// max line width of the body built so far. Version segment is omitted
	// when h.version is empty.
	body := b.String()
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

	b.WriteString("\n")
	b.WriteString(helpFaint.Render(footer))
	return helpBorder.Render(b.String())
}
