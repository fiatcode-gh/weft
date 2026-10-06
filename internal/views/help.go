package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Help is a stateless overlay listing the keymap. ? or esc toggles it off.
type Help struct {
	version string
	width   int
	height  int
}

// NewHelp builds the help overlay at the current terminal size. width clamps
// rendered lines and decides single- vs two-column layout; height caps the
// panel so it can never overflow the screen — body lines past the cap are
// dropped with a faint "resize" hint, while the title and close hint always
// stay visible. Passing width or height as 0 disables that dimension's
// constraint, yielding the natural content-sized panel (used by tests).
func NewHelp(version string, width, height int) *Help {
	return &Help{version: version, width: width, height: height}
}

// SetSize updates the cached terminal size.
func (h *Help) SetSize(w, ht int) { h.width, h.height = w, ht }

// Update reports whether the overlay should close. There's no state to mutate.
func (h *Help) Update(key string) OverlayResult {
	switch key {
	case keyEsc, "?", keyQ:
		return overlayCancel()
	}
	return OverlayResult{}
}

var (
	helpBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2)
	helpKey     = lipgloss.NewStyle().Bold(true).Foreground(colorHighlight)
	helpSection = lipgloss.NewStyle().Faint(true).Italic(true)
)

const (
	// helpKeyCol is the column descriptions start at, measured from the start
	// of the key (after the 2-space row indent).
	helpKeyCol = 10
	// helpColGutter is the blank gap between the two key columns.
	helpColGutter = 3
	// helpChrome is the horizontal cells the border + padding add (2 border +
	// 2×2 padding), used to turn a width budget into a content budget.
	helpChrome = 6
)

type helpRow struct{ key, desc string }

type helpGroup struct {
	title string
	rows  []helpRow
}

var helpGroups = []helpGroup{
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
		{"b", "backlinks + unlinked refs for the current page"},
		{"T", "open todos dashboard"},
	}},
	{"Sync", []helpRow{
		{"S", "commit, pull & push the graph (git)"},
	}},
	{"Edit", []helpRow{
		{"e", "edit current page in-app"},
		{"E", "edit current page in $EDITOR"},
		{"ctrl-s", "save; merges outside edits, asks on a clash"},
		{"pgup/pgdn", "page up / down (while editing)"},
		{"enter", "continue list bullet (empty bullet ends it)"},
		{"ctrl-t", "cycle TODO / DONE on the current bullet"},
		{"tab", "indent the current line"},
		{"shift-tab", "de-indent the current line"},
		{"esc", "leave editor (prompts if unsaved)"},
		{"[[", "start wiki-link; pick a page to complete"},
		{"↑ / ↓", "choose completion (list open)"},
		{"enter/tab", "insert selected link"},
		{"esc", "dismiss completion list"},
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
	body := h.renderBody()
	header := styleTitle.Render("Keys")

	// Footer aligns to the body width, so measure the body before any clip.
	contentWidth := 0
	for _, line := range strings.Split(body, "\n") {
		if w := lipgloss.Width(line); w > contentWidth {
			contentWidth = w
		}
	}
	footer := h.footer(contentWidth)

	// Height guard: when the terminal height is known, drop body lines that
	// wouldn't fit so the panel never overflows (and the close hint never
	// scrolls off). Fixed chrome around the body is 5 lines: border top and
	// bottom (2), the title (1), and the blank lines after the title and
	// before the footer (2).
	if h.height > 0 {
		footerLines := strings.Count(footer, "\n") + 1
		budget := h.height - footerLines - 5
		if budget < 1 {
			budget = 1
		}
		lines := strings.Split(body, "\n")
		if len(lines) > budget {
			indicator := styleFaint.Render("… resize terminal to see all keys")
			if h.width > helpChrome {
				indicator = clamp(indicator, h.width-helpChrome)
			}
			lines = append(lines[:budget-1], indicator)
			body = strings.Join(lines, "\n")
		}
	}

	inner := header + "\n\n" + body + "\n\n" + styleFaint.Render(footer)
	return helpBorder.Render(inner)
}

// renderBody lays the key groups out in two balanced columns when the width
// allows it, otherwise a single column clamped to the width.
func (h *Help) renderBody() string {
	left, right := splitGroups(helpGroups)
	leftBlock := renderColumn(left)
	rightBlock := renderColumn(right)

	twoColWidth := lipgloss.Width(leftBlock) + helpColGutter + lipgloss.Width(rightBlock)
	if h.width <= 0 || twoColWidth+helpChrome <= h.width {
		gutter := lipgloss.NewStyle().MarginRight(helpColGutter).Render(leftBlock)
		return lipgloss.JoinHorizontal(lipgloss.Top, gutter, rightBlock)
	}

	// Too narrow for two columns: stack every group and clamp to the width.
	body := renderColumn(helpGroups)
	if h.width > helpChrome {
		max := h.width - helpChrome
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			lines[i] = clamp(line, max)
		}
		body = strings.Join(lines, "\n")
	}
	return body
}

// footer renders the close hint on the left and the version on the right,
// padded to contentWidth. It wraps to two lines when they can't fit inline,
// and omits the version segment entirely when version is empty.
func (h *Help) footer(contentWidth int) string {
	left := "? or esc to close"
	if h.version == "" {
		return left
	}
	right := "weft " + h.version
	gap := contentWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left + "\n" + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderColumn stacks groups vertically with one blank line between them.
func renderColumn(groups []helpGroup) string {
	blocks := make([]string, len(groups))
	for i, g := range groups {
		blocks[i] = renderGroup(g)
	}
	return strings.Join(blocks, "\n\n")
}

// renderGroup renders one group: a faint title then its key/description rows.
func renderGroup(g helpGroup) string {
	var b strings.Builder
	b.WriteString(helpSection.Render(g.title))
	for _, r := range g.rows {
		pad := helpKeyCol - lipgloss.Width(r.key)
		if pad < 1 {
			pad = 1
		}
		b.WriteString("\n  ")
		b.WriteString(helpKey.Render(r.key))
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(r.desc)
	}
	return b.String()
}

// splitGroups divides groups into two columns balanced by line count, keeping
// the original order and never leaving the second column empty.
func splitGroups(groups []helpGroup) (left, right []helpGroup) {
	total := 0
	for _, g := range groups {
		total += 1 + len(g.rows)
	}
	half := total / 2
	acc, split := 0, 0
	for split < len(groups)-1 {
		gh := 1 + len(groups[split].rows)
		if acc+gh >= half {
			split++
			break
		}
		acc += gh
		split++
	}
	return groups[:split], groups[split:]
}
