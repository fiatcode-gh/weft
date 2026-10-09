package views

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// Agenda is the overlay listing open tasks that are overdue, due today or due
// within the next graph.AgendaDays days. today is fixed at construction, so the
// view never shifts under the user mid-session.
type Agenda struct {
	listBox
	idx    *graph.Index
	today  time.Time
	items  []graph.AgendaItem
	done   doneRows // tasks marked DONE this session, kept listed so they can be undone
	errMsg string   // failure feedback, rendered inside the panel
}

func NewAgenda(idx *graph.Index, today time.Time, width, height int) *Agenda {
	a := &Agenda{listBox: listBox{width: width, height: height}, idx: idx, today: today, done: doneRows{}}
	a.recompute(taskKey{}, false)
	return a
}

// recompute rebuilds items from the index plus the session's done rows and
// reselects the task at selKey when hadSel and it is still listed.
func (a *Agenda) recompute(selKey taskKey, hadSel bool) {
	todos := make([]graph.TodoBullet, 0, len(a.idx.Todos)+len(a.done))
	for _, b := range a.idx.Todos {
		if !a.done.shadows(b) {
			todos = append(todos, b)
		}
	}
	todos = append(todos, slices.Collect(maps.Values(a.done))...)
	a.items = graph.BuildAgenda(todos, a.today)
	if hadSel {
		for i, it := range a.items {
			if (taskKey{it.Todo.Page, it.Todo.LineNumber}) == selKey {
				a.sel = i
				return
			}
		}
	}
	a.sel = min(a.sel, max(0, len(a.items)-1))
}

// SetError records a message to show inside the panel, above the footer.
func (a *Agenda) SetError(msg string) { a.errMsg = msg }

func (a *Agenda) selectedKey() (taskKey, bool) {
	if a.sel < 0 || a.sel >= len(a.items) {
		return taskKey{}, false
	}
	b := a.items[a.sel].Todo
	return taskKey{b.Page, b.LineNumber}, true
}

// marked folds a successful write of m (the task now sits on line) into the
// panel, with the same semantics as Todos.marked.
func (a *Agenda) marked(m taskMark, line int, idx *graph.Index) {
	selKey, hadSel := a.selectedKey()
	from := taskKey{m.page, m.line}
	to := taskKey{m.page, line}
	if m.to == "DONE" {
		for _, it := range a.items {
			if it.Todo.Page == from.page && it.Todo.LineNumber == from.line {
				b := it.Todo
				b.LineNumber = line
				a.done[to] = b
				break
			}
		}
	} else {
		delete(a.done, from)
	}
	if hadSel && selKey == from {
		selKey = to
	}
	a.idx = idx
	a.recompute(selKey, hadSel)
}

func (a *Agenda) Update(key string) OverlayResult {
	a.errMsg = ""
	switch key {
	case keyEsc, keyQ:
		return overlayCancel()
	case keyUp, keyK:
		a.moveUp()
	case keyDown, keyJ:
		a.moveDown(len(a.items))
	case "x":
		if a.sel < 0 || a.sel >= len(a.items) {
			break
		}
		b := a.items[a.sel].Todo
		m := taskMark{page: b.Page, line: b.LineNumber, from: b.Marker, to: "DONE", tail: graph.TaskTail(b.Priority, b.Text)}
		if _, isDone := a.done[taskKey{b.Page, b.LineNumber}]; isDone {
			m.from, m.to = "DONE", b.Marker
		}
		return overlayMarkTask(m)
	case keyEnter:
		if a.sel < 0 || a.sel >= len(a.items) {
			break
		}
		b := a.items[a.sel].Todo
		if _, isDone := a.done[taskKey{b.Page, b.LineNumber}]; isDone {
			// The task is no longer an open todo, so its ordinal is gone.
			return overlayOpen(b.Page)
		}
		return overlayOpenTask(b.Page, b.Ordinal)
	}
	return OverlayResult{}
}

// agendaInnerWidthMax lets wide terminals show a full "scheduled … · deadline … ·
// page" label next to the task text; the shared 80-cell cap truncates it.
const agendaInnerWidthMax = 92

// innerWidth overrides listBox.innerWidth with the wider agenda cap.
func (a *Agenda) innerWidth() int {
	return clampInt(a.width-2-4-4, listInnerWidthMin, agendaInnerWidthMax)
}

// visibleRows is the terminal-row budget for the scrollable area; same chrome
// as Todos.
func (a *Agenda) visibleRows() int {
	const chrome = 11
	return clampInt(a.height-chrome, listVisibleRowsMin, todosVisibleRowsMax)
}

func (a *Agenda) View() string {
	inner := a.innerWidth()
	var sb strings.Builder
	sb.WriteString(styleTitle.Render("Agenda"))
	sb.WriteString(styleFaint.Render(fmt.Sprintf("   · today %s   (%d)", a.today.Format("2006-01-02 Mon"), len(a.items))))
	sb.WriteString("\n\n")
	sb.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	sb.WriteString("\n")
	if len(a.items) == 0 {
		sb.WriteString(styleFaint.Render(clamp("  nothing due — no open task is overdue, due today or in the next 7 days", inner)))
		sb.WriteString("\n")
		a.writeFooter(&sb, inner)
		return renderBordered(inner+4, sb.String())
	}

	section := func(i int) string { return a.items[i].Section.String() }
	start, end := groupedWindow(len(a.items), a.sel, a.visibleRows(), section)
	if start > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↑ %d more above", start)))
		sb.WriteString("\n")
	}

	lastSection := ""
	for i := start; i < end; i++ {
		it := a.items[i]
		if s := section(i); s != lastSection {
			if lastSection != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(todosGroup.Render(s))
			sb.WriteString("\n")
			lastSection = s
		}
		rowBudget := inner - 3 // marker prefix is 3 cells
		marker := "   "
		if i == a.sel {
			marker = styleSel.Render(" ▶ ")
		}
		_, isDone := a.done[taskKey{it.Todo.Page, it.Todo.LineNumber}]
		sb.WriteString(marker)
		sb.WriteString(taskRow(it.Todo, isDone, i == a.sel, taskDates(it.Todo)+" · "+it.Todo.Page, rowBudget))
		sb.WriteString("\n")
	}

	if below := len(a.items) - end; below > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↓ %d more below", below)))
		sb.WriteString("\n")
	}

	a.writeFooter(&sb, inner)
	return renderBordered(inner+4, sb.String())
}

// writeFooter writes the blank separator, the pending error (if any) and the
// key legend.
func (a *Agenda) writeFooter(sb *strings.Builder, inner int) {
	sb.WriteString("\n")
	if a.errMsg != "" {
		sb.WriteString(styleTitle.Render(clamp(a.errMsg, inner)))
		sb.WriteString("\n")
	}
	sb.WriteString(styleFaint.Render(clamp("↑/↓ select · x done/undo · enter open · esc back", inner)))
}
