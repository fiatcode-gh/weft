package views

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// viewPos is one display row of the editor: row Row of buffer line Line. Line 0
// also owns Row -1, the blank top margin the read view has; Row < -1 on line 0
// is extra blank rows above it, used only to open the editor with line 0
// exactly where the read view showed it.
type viewPos struct{ line, row int }

func (p viewPos) less(q viewPos) bool {
	if p.line != q.line {
		return p.line < q.line
	}
	return p.row < q.row
}

// lineCache is what the frame needs of one buffer line: its block info and
// wrapped rows, and its spans once painted. An entry is valid while the
// line's text and info are unchanged.
type lineCache struct {
	text    string
	info    render.LineInfo
	rows    []render.Row
	spans   []render.Span
	painted bool
}

// textHeight is the number of text rows: the terminal height less the status
// line and the completion strip, at least one.
func (e *EditorView) textHeight() int {
	return max(1, e.height-1-e.completer.rows())
}

// syncBuffer tells the scanner and the cache about edits since the last call.
func (e *EditorView) syncBuffer() {
	from, ok := e.buf.Changed()
	if !ok {
		return
	}
	e.scanner.Invalidate(from)
	// A fence's highlighting runs across its lines, so a changed line can
	// recolour its neighbours without changing their text.
	for _, c := range e.cache {
		if c.info.Kind == render.KindFence || c.info.Kind == render.KindCode {
			c.painted = false
		}
	}
}

// line returns the cache entry of buffer line i, wrapping it if needed.
func (e *EditorView) line(i int) *lineCache {
	e.syncBuffer()
	text := e.buf.Line(i)
	info := e.scanner.Info(e.buf, i)
	c := e.cache[i]
	if c == nil || c.text != text || c.info != info {
		c = &lineCache{text: text, info: info, rows: e.geo.Rows(text, info)}
		e.cache[i] = c
		e.stats.wrapped++
	}
	return c
}

// next is the display row after p; false at the last row of the buffer.
func (e *EditorView) next(p viewPos) (viewPos, bool) {
	switch {
	case p.row < -1:
		return viewPos{p.line, p.row + 1}, true
	case p.row == -1:
		return viewPos{p.line, 0}, true
	case p.row+1 < len(e.line(p.line).rows):
		return viewPos{p.line, p.row + 1}, true
	case p.line+1 < e.buf.Len():
		return viewPos{p.line + 1, 0}, true
	}
	return p, false
}

// prev is the display row before p; false at the margin row. With virtual,
// rows above the margin exist on line 0.
func (e *EditorView) prev(p viewPos, virtual bool) (viewPos, bool) {
	switch {
	case p.line == 0 && p.row <= -1:
		return viewPos{0, p.row - 1}, virtual
	case p.row > 0:
		return viewPos{p.line, p.row - 1}, true
	case p.line == 0:
		return viewPos{0, -1}, true
	}
	return viewPos{p.line - 1, len(e.line(p.line-1).rows) - 1}, true
}

// cursorRow is the display row holding the cursor and the screen column it
// sits at.
func (e *EditorView) cursorRow() (viewPos, int) {
	cur := e.buf.Cursor()
	c := e.line(cur.Line)
	row, x := e.geo.CellX(c.text, c.info, c.rows, cur.Col)
	return viewPos{cur.Line, row}, x
}

// normalizeTop keeps top on a row that exists after the buffer or the width
// changed under it.
func (e *EditorView) normalizeTop() {
	t := &e.top
	if t.line >= e.buf.Len() {
		*t = viewPos{e.buf.Len() - 1, 0}
	}
	if t.row >= 0 {
		t.row = min(t.row, len(e.line(t.line).rows)-1)
	}
}

// ensureVisible scrolls the minimum that puts the cursor on screen. Going up to
// the first row of line 0 reveals its margin row.
func (e *EditorView) ensureVisible() {
	e.normalizeTop()
	cur, _ := e.cursorRow()
	if cur.less(e.top) {
		e.top = cur
		if cur == (viewPos{0, 0}) {
			e.top = viewPos{0, -1}
		}
		return
	}
	bottom := e.top
	for range e.textHeight() - 1 {
		n, ok := e.next(bottom)
		if !ok {
			break
		}
		bottom = n
	}
	if !bottom.less(cur) {
		return
	}
	e.top = e.rowsBefore(cur, e.textHeight()-1)
}

// rowsBefore is the display row n rows above p (fewer at the margin row).
func (e *EditorView) rowsBefore(p viewPos, n int) viewPos {
	for range n {
		q, ok := e.prev(p, false)
		if !ok {
			break
		}
		p = q
	}
	return p
}

// cursorScreenRow is the screen row of the cursor's display row, 0 if it is
// not in the window.
func (e *EditorView) cursorScreenRow() int {
	cur, _ := e.cursorRow()
	p := e.top
	for y := range e.textHeight() {
		if p == cur {
			return y
		}
		n, ok := e.next(p)
		if !ok {
			break
		}
		p = n
	}
	return 0
}

// scrollCursorTo puts the cursor's display row on screen row sr.
func (e *EditorView) scrollCursorTo(sr int) {
	cur, _ := e.cursorRow()
	t := cur
	for range min(sr, e.textHeight()-1) {
		q, ok := e.prev(t, false)
		if !ok {
			break
		}
		t = q
	}
	e.top = t
	e.ensureVisible()
}

// place puts the cursor at the start of row RowInLine of line at.Line and
// scrolls that row onto screen row at.ScreenRow, with rows above line 0 blank.
func (e *EditorView) place(at Anchor) {
	line := clampInt(at.Line, 0, e.buf.Len()-1)
	c := e.line(line)
	k := clampInt(at.RowInLine, 0, len(c.rows)-1)
	e.buf.MoveTo(buffer.Pos{Line: line, Col: c.rows[k].Start}, false)
	t := viewPos{line, k}
	for range clampInt(at.ScreenRow, 0, e.textHeight()-1) {
		q, _ := e.prev(t, true)
		t = q
	}
	e.top = t
}

// ExitAnchor is where the cursor sits now: its buffer line, the index of its
// row within that line, and the screen row it is drawn on. The App places the
// read view with it when the editor closes.
func (e *EditorView) ExitAnchor() Anchor {
	cur, _ := e.cursorRow()
	return Anchor{Line: cur.line, RowInLine: cur.row, ScreenRow: e.cursorScreenRow()}
}

// moveRows moves the cursor n display rows up (dir < 0) or down at the
// remembered screen column. A single step off the first or last row goes to
// the buffer start or end; a longer move (a page) clamps to that row.
func (e *EditorView) moveRows(dir, n int) {
	cur, x := e.cursorRow()
	if !e.goalOK {
		e.goalX, e.goalOK = x, true
	}
	p, moved := cur, 0
	for moved < n {
		var q viewPos
		var ok bool
		if dir < 0 {
			q, ok = e.prev(p, false)
			ok = ok && q.row >= 0
		} else {
			q, ok = e.next(p)
		}
		if !ok {
			break
		}
		p = q
		moved++
	}
	if moved == 0 && n == 1 {
		e.buf.Break()
		if dir < 0 {
			e.buf.MoveTo(buffer.Pos{}, false)
		} else {
			e.buf.MoveTo(e.buf.End(), false)
		}
		return
	}
	c := e.line(p.line)
	e.buf.MoveTo(buffer.Pos{Line: p.line, Col: e.geo.OffsetAt(c.text, c.info, c.rows, p.row, e.goalX)}, false)
}

// frameRows draws the text window: textHeight rows, "" where blank.
func (e *EditorView) frameRows() []string {
	e.normalizeTop()
	h := e.textHeight()
	out := make([]string, h)
	p := e.top
	lo, hi := p.line, p.line
	for y := range h {
		if p.row >= 0 {
			out[y] = e.drawRow(p)
			hi = p.line
		}
		n, ok := e.next(p)
		if !ok {
			break
		}
		p = n
	}
	for i := range e.cache {
		if i < lo || i > hi {
			delete(e.cache, i)
		}
	}
	return out
}

// drawRow draws one display row with row.Col's leading padding.
func (e *EditorView) drawRow(p viewPos) string {
	c := e.line(p.line)
	if !c.painted {
		c.spans = e.painter.Paint(e.buf, p.line, c.info, e.scanner)
		c.painted = true
		e.stats.painted++
	}
	r := c.rows[p.row]
	s := render.DrawRow(c.text, c.info, r, c.spans)
	if s == "" {
		return ""
	}
	return strings.Repeat(" ", r.Col) + s
}

// View renders the text window, the completion strip and the status line.
func (e *EditorView) View() string {
	e.ensureVisible()
	v := strings.Join(e.frameRows(), "\n")
	pad := e.geo.Margin
	if strip := e.completer.View(max(1, e.width-pad)); strip != "" {
		v += "\n" + indentBlock(strip, pad)
	}
	return v + "\n" + indentBlock(e.statusLine(), pad)
}

// Cursor is where the terminal cursor goes, or nil while a prompt owns the keys.
func (e *EditorView) Cursor() *tea.Cursor {
	if e.mode != editing {
		return nil
	}
	e.ensureVisible()
	cur, x := e.cursorRow()
	p := e.top
	for y := range e.textHeight() {
		if p == cur {
			return tea.NewCursor(min(x, max(0, e.width-1)), y)
		}
		n, ok := e.next(p)
		if !ok {
			break
		}
		p = n
	}
	return nil
}
