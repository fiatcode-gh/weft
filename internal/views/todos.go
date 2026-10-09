package views

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

type Todos struct {
	listBox
	idx     *graph.Index
	filter  string // "", "TODO", "LATER", "DOING", "WAITING"
	visible []graph.TodoBullet
	done    doneRows // tasks marked DONE this session, kept listed so they can be undone
	errMsg  string   // failure feedback, rendered inside the panel
}

func NewTodos(idx *graph.Index, width, height int) *Todos {
	t := &Todos{listBox: listBox{width: width, height: height}, idx: idx, done: doneRows{}}
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
		if t.done.shadows(b) {
			continue
		}
		t.visible = append(t.visible, b)
	}
	for _, b := range t.done {
		t.visible = append(t.visible, b)
	}
	if t.filter != "" {
		kept := t.visible[:0]
		for _, b := range t.visible {
			if b.Marker == t.filter {
				kept = append(kept, b)
			}
		}
		t.visible = kept
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

// SetError records a message to show inside the panel, above the footer.
func (t *Todos) SetError(msg string) { t.errMsg = msg }

// selectedKey is the page+line identity of the selected row.
func (t *Todos) selectedKey() (taskKey, bool) {
	if t.sel < 0 || t.sel >= len(t.visible) {
		return taskKey{}, false
	}
	b := t.visible[t.sel]
	return taskKey{b.Page, b.LineNumber}, true
}

// marked folds a successful write of m (the task now sits on line) into the
// panel: a task marked DONE stays listed as a done row, an undone one returns
// from the fresh index, and the selection and filter survive the rebuild.
func (t *Todos) marked(m taskMark, line int, idx *graph.Index) {
	oldSel := t.sel
	selKey, hadSel := t.selectedKey()
	from := taskKey{m.page, m.line}
	to := taskKey{m.page, line}
	if m.to == "DONE" {
		for _, b := range t.visible {
			if b.Page == from.page && b.LineNumber == from.line {
				b.LineNumber = line
				t.done[to] = b
				break
			}
		}
	} else {
		delete(t.done, from)
	}
	if hadSel && selKey == from {
		selKey = to
	}
	t.idx = idx
	t.recompute()
	t.sel = min(oldSel, max(0, len(t.visible)-1))
	if hadSel {
		for i, b := range t.visible {
			if (taskKey{b.Page, b.LineNumber}) == selKey {
				t.sel = i
				break
			}
		}
	}
}

func (t *Todos) Update(key string) OverlayResult {
	t.errMsg = ""
	switch key {
	case keyEsc, keyQ:
		return overlayCancel()
	case "t":
		t.cycleFilter()
	case keyUp, keyK:
		t.moveUp()
	case keyDown, keyJ:
		t.moveDown(len(t.visible))
	case "x":
		if t.sel < 0 || t.sel >= len(t.visible) {
			break
		}
		b := t.visible[t.sel]
		m := taskMark{page: b.Page, line: b.LineNumber, from: b.Marker, to: "DONE", tail: graph.TaskTail(b.Priority, b.Text)}
		if _, isDone := t.done[taskKey{b.Page, b.LineNumber}]; isDone {
			m.from, m.to = "DONE", b.Marker
		}
		return overlayMarkTask(m)
	case keyEnter:
		if t.sel < 0 || t.sel >= len(t.visible) {
			break
		}
		b := t.visible[t.sel]
		if _, isDone := t.done[taskKey{b.Page, b.LineNumber}]; isDone {
			// The task is no longer an open todo, so its ordinal is gone.
			return overlayOpen(b.Page)
		}
		return overlayOpenTask(b.Page, b.Ordinal)
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
	return groupedWindow(len(t.visible), t.sel, t.visibleRows(), func(i int) string { return t.visible[i].Page })
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
		t.writeFooter(&sb, inner)
		return renderBordered(inner+4, sb.String())
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
		// Clamp the row content to one line. Without this, a long todo
		// wraps inside the panel without hanging indent — continuation
		// lines start flush at column 0 and look like sibling bullets,
		// and only the first line of a wrapped selected row carries the
		// ▶ marker. Matches the picker/search/backlinks single-line policy.
		rowBudget := inner - 3 // marker prefix is 3 cells
		marker := "   "        // 3-cell to match selected " ▶ " width
		if i == t.sel {
			marker = styleSel.Render(" ▶ ")
		}
		_, isDone := t.done[taskKey{b.Page, b.LineNumber}]
		sb.WriteString(marker)
		sb.WriteString(taskRow(b, isDone, i == t.sel, taskDates(b), rowBudget))
		sb.WriteString("\n")
	}

	if below > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↓ %d more below", below)))
		sb.WriteString("\n")
	}

	t.writeFooter(&sb, inner)
	return renderBordered(inner+4, sb.String())
}

// writeFooter writes the blank separator, the pending error (if any) and the
// key legend, the way Picker does.
func (t *Todos) writeFooter(sb *strings.Builder, inner int) {
	sb.WriteString("\n")
	if t.errMsg != "" {
		sb.WriteString(styleTitle.Render(clamp(t.errMsg, inner)))
		sb.WriteString("\n")
	}
	sb.WriteString(styleFaint.Render(clamp("↑/↓ select · x done/undo · t cycle filter · enter open · esc back", inner)))
}
