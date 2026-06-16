package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
)

type Todos struct {
	listBox
	idx     *graph.Index
	filter  string // "", "TODO", "LATER", "DOING", "WAITING"
	visible []graph.TodoBullet
}

func NewTodos(idx *graph.Index, width, height int) *Todos {
	t := &Todos{listBox: listBox{width: width, height: height}, idx: idx}
	t.recompute()
	return t
}

var markerCycle = []string{"", "TODO", "LATER", "DOING", "WAITING"}

func (t *Todos) cycleFilter() {
	for i, m := range markerCycle {
		if m == t.filter {
			t.filter = markerCycle[(i+1)%len(markerCycle)]
			t.recompute()
			return
		}
	}
}

func (t *Todos) recompute() {
	t.visible = t.visible[:0]
	for _, b := range t.idx.Todos {
		if t.filter != "" && b.Marker != t.filter {
			continue
		}
		t.visible = append(t.visible, b)
	}
	sort.SliceStable(t.visible, func(i, j int) bool {
		if t.visible[i].Page != t.visible[j].Page {
			return t.visible[i].Page < t.visible[j].Page
		}
		return t.visible[i].LineNumber < t.visible[j].LineNumber
	})
	if t.sel >= len(t.visible) {
		t.sel = 0
	}
}

func (t *Todos) Update(key string) OverlayResult {
	switch key {
	case keyEsc, keyQ:
		return OverlayResult{Cancel: true}
	case "t":
		t.cycleFilter()
	case keyUp, keyK:
		t.moveUp()
	case keyDown, keyJ:
		t.moveDown(len(t.visible))
	case keyEnter:
		if t.sel >= 0 && t.sel < len(t.visible) {
			return OverlayResult{
				Selected:    t.visible[t.sel].Page,
				TaskOrdinal: t.visible[t.sel].Ordinal,
				DeepLink:    true,
				Accept:      true,
			}
		}
	}
	return OverlayResult{}
}

var todosGroup = lipgloss.NewStyle().Bold(true).Foreground(colorHighlight)

// todosMark colors each workflow marker. Mirrors the task-marker palette in
// internal/render/page.go; intentionally not shared across the package
// boundary (see theme.go).
var todosMark = map[string]lipgloss.Style{
	"TODO":    lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true), // red
	"DOING":   lipgloss.NewStyle().Foreground(colorCursor).Bold(true),         // yellow
	"LATER":   lipgloss.NewStyle().Foreground(colorHighlight).Bold(true),      // blue
	"WAITING": lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true), // dim
}

const todosVisibleRowsMax = 16

// visibleRows returns the terminal-row budget available for the scrollable
// bullet+header area. Group headers and inter-group blank lines count
// against this budget too — see computeWindow.
func (t *Todos) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) + 1 (divider)
	// + 1 (blank) + 1 (hint) ≈ 9 lines. Add 2 more to leave headroom for
	// scroll-position hints when they're shown.
	const chrome = 11
	return clampInt(t.height-chrome, listVisibleRowsMin, todosVisibleRowsMax)
}

// computeWindow chooses a [start, end) slice of t.visible to display so
// that t.sel is always inside it, accounting for the row cost of group
// headers and inter-group blank lines (1 row each).
func (t *Todos) computeWindow() (start, end int) {
	n := len(t.visible)
	if n == 0 {
		return 0, 0
	}
	budget := t.visibleRows()
	if budget <= 0 {
		return 0, 0
	}

	// Per-position incremental row cost when i follows prevPage at i-1.
	// Returns the rows the i-th bullet adds: 1 for the bullet, +1 if it
	// starts a new group (header), +1 for the blank line before the new
	// group when prevPage is non-empty.
	cost := func(i int, prevPage string) int {
		c := 1
		if t.visible[i].Page != prevPage {
			c++
			if prevPage != "" {
				c++
			}
		}
		return c
	}

	walkForward := func(s int) int {
		used := 0
		prev := ""
		e := s
		for i := s; i < n; i++ {
			c := cost(i, prev)
			if used+c > budget {
				break
			}
			used += c
			prev = t.visible[i].Page
			e = i + 1
		}
		return e
	}

	// Centre the selection in the window. If t.sel ends up past the
	// rendered end (because the chosen start left too little budget),
	// nudge start forward until t.sel fits — guaranteed to terminate
	// because start can rise to t.sel.
	start = t.sel - budget/2
	if start < 0 {
		start = 0
	}
	end = walkForward(start)
	for end <= t.sel && start < t.sel {
		start++
		end = walkForward(start)
	}
	return start, end
}

func (t *Todos) View() string {
	inner := t.innerWidth()
	var sb strings.Builder
	filterTxt := "all"
	if t.filter != "" {
		filterTxt = t.filter
	}
	sb.WriteString(styleTitle.Render("Open todos"))
	sb.WriteString(styleFaint.Render(fmt.Sprintf("   · filter: %s   (%d)", filterTxt, len(t.visible))))
	sb.WriteString("\n\n")
	sb.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	sb.WriteString("\n")
	if len(t.visible) == 0 {
		sb.WriteString(styleFaint.Render("  nothing open"))
		sb.WriteString("\n")
		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render(clamp("↑/↓ select · t cycle filter · enter open · esc back", inner)))
		return styleBorder.Width(inner + 4).Render(sb.String())
	}

	start, end := t.computeWindow()
	above := start
	below := len(t.visible) - end

	if above > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↑ %d more above", above)))
		sb.WriteString("\n")
	}

	lastPage := ""
	for i := start; i < end; i++ {
		b := t.visible[i]
		if b.Page != lastPage {
			if lastPage != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(todosGroup.Render(clamp(b.Page, inner)))
			sb.WriteString("\n")
			lastPage = b.Page
		}
		prio := ""
		if b.Priority != "" {
			prio = "[#" + b.Priority + "] "
		}
		// Clamp the row content to one line. Without this, a long todo
		// wraps inside the panel without hanging indent — continuation
		// lines start flush at column 0 and look like sibling bullets,
		// and only the first line of a wrapped selected row carries the
		// ▶ marker. Matches the picker/search/backlinks single-line policy.
		rowBudget := inner - 3 // marker prefix is 3 cells
		marker := "   "        // 3-cell to match selected " ▶ " width
		var row string
		if i == t.sel {
			marker = styleSel.Render(" ▶ ")
			row = styleSel.Render(clamp(fmt.Sprintf("%s %s%s", b.Marker, prio, b.Text), rowBudget))
		} else {
			styledMarker := b.Marker
			if st, ok := todosMark[b.Marker]; ok {
				styledMarker = st.Render(b.Marker)
			}
			row = clamp(styledMarker+" "+prio+b.Text, rowBudget)
		}
		sb.WriteString(marker)
		sb.WriteString(row)
		sb.WriteString("\n")
	}

	if below > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↓ %d more below", below)))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(styleFaint.Render(clamp("↑/↓ select · t cycle filter · enter open · esc back", inner)))
	return styleBorder.Width(inner + 4).Render(sb.String())
}
