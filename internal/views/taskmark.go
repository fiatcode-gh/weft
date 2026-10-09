package views

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// taskMark is one request to flip a task's marker on disk: the task is the one
// on page whose tail is tail, found near line (1-based), currently marked from.
type taskMark struct {
	page     string
	line     int
	from, to string
	tail     string
}

// taskPanel is an overlay that lists tasks and takes part in the `x` write
// path: the App reports failures in the panel and hands it the refreshed
// index after a successful write.
type taskPanel interface {
	Overlay
	SetError(msg string)
	marked(m taskMark, line int, idx *graph.Index)
}

// taskKey identifies a task row by page and 1-based line.
type taskKey struct {
	page string
	line int
}

// doneRows holds the tasks marked DONE during one overlay session, as they
// were (Marker is the earlier marker), so they stay listed and can be undone.
type doneRows map[taskKey]graph.TodoBullet

// shadows reports whether the done row at fresh's key stands for fresh itself
// (same tail), so the fresh row is not listed twice. A done row whose key now
// holds a different open task is stale after an outside line shift: it is
// dropped and fresh stays visible.
func (d doneRows) shadows(fresh graph.TodoBullet) bool {
	k := taskKey{fresh.Page, fresh.LineNumber}
	row, ok := d[k]
	if !ok {
		return false
	}
	if graph.TaskTail(row.Priority, row.Text) == graph.TaskTail(fresh.Priority, fresh.Text) {
		return true
	}
	delete(d, k)
	return false
}

var doneStyle = lipgloss.NewStyle().Strikethrough(true).Faint(true)

// taskDates renders a task's stamps as "scheduled D[ T] · deadline D[ T]";
// "" when it has none.
func taskDates(b graph.TodoBullet) string {
	part := func(label string, s graph.Stamp) string {
		if s.Date == "" {
			return ""
		}
		out := label + " " + s.Date
		if s.Time != "" {
			out += " " + s.Time
		}
		return out
	}
	var parts []string
	for _, p := range []string{part("scheduled", b.Scheduled), part("deadline", b.Deadline)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// taskRow renders one task row of at most budget cells. The text is clamped
// first when there is a suffix, so the faint suffix stays visible.
func taskRow(b graph.TodoBullet, done, selected bool, suffix string, budget int) string {
	marker := b.Marker
	if done {
		marker = "DONE"
	}
	prio := ""
	if b.Priority != "" {
		prio = "[#" + b.Priority + "] "
	}
	text := b.Text
	if suffix != "" {
		room := max(10, budget-ansi.StringWidth(marker+" "+prio)-ansi.StringWidth(suffix)-2)
		text = clamp(text, room)
	}
	content := marker + " " + prio + text
	var row string
	switch {
	case selected:
		st := styleSel
		if done {
			st = st.Strikethrough(true)
		}
		row = st.Render(content)
	case done:
		row = doneStyle.Render(content)
	default:
		styled := marker
		if st, ok := todosMark[marker]; ok {
			styled = st.Render(marker)
		}
		row = styled + " " + prio + text
	}
	if suffix != "" {
		row += "  " + styleFaint.Render(suffix)
	}
	return clamp(row, budget)
}

// markTask writes m to disk through the guarded snapshot path, reindexes, and
// refreshes the panel. Failures render inside the panel, where the user is
// looking; the status bar is hidden behind the overlay.
func (a *App) markTask(m taskMark) tea.Cmd {
	p, _ := a.active.(taskPanel)
	fail := func(reason string) tea.Cmd {
		if p != nil {
			p.SetError("not marked: " + reason)
		}
		return nil
	}
	meta, ok := a.idx.ByName[m.page]
	if !ok {
		return fail(m.page + " is no longer in the index")
	}
	snap, err := a.readSnapshot(meta.Path)
	if err != nil {
		return fail("cannot read " + m.page + ": " + err.Error())
	}
	if !snap.Exists {
		return fail("cannot read " + m.page + ": file not found")
	}
	out, line, err := graph.MarkTask(snap.Content, m.line, m.from, m.to, m.tail)
	if err != nil {
		if errors.Is(err, graph.ErrTaskNotFound) {
			return fail("task not found in " + m.page + " — nothing written")
		}
		return fail(err.Error())
	}
	if err := edit.WriteFileIfUnchanged(meta.Path, snap, []byte(out)); err != nil {
		if errors.Is(err, edit.ErrChanged) {
			return fail(m.page + " changed on disk — try again")
		}
		return fail("write failed: " + err.Error())
	}
	if err := a.reindex(); err != nil {
		return fail("reindex failed: " + err.Error())
	}
	if p != nil {
		p.marked(m, line, a.idx)
	}
	return a.statusProbeCmd()
}
