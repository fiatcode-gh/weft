package views

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

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

// textHeight is the number of text rows: the terminal height less the rule
// row, the status line, the completion strip, the find bar and the date prompt, at least one. With no strip
// it is the read view's window, so switching views keeps every row in place.
func (e *EditorView) textHeight() int {
	return max(1, e.height-2-e.completer.rows()-e.findRows()-e.promptRows())
}

// syncBuffer tells the scanner and the cache about edits since the last call.
func (e *EditorView) syncBuffer() {
	from, ok := e.buf.Changed()
	if !ok {
		return
	}
	e.scanner.Invalidate(from)
	e.preview.Invalidate(from)
	clear(e.previewed)
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

// next is the display row after p; false at the last row of the buffer. Lines
// without rows are skipped.
func (e *EditorView) next(p viewPos) (viewPos, bool) {
	switch {
	case p.row < -1:
		return viewPos{p.line, p.row + 1}, true
	case p.row == -1:
		if q, ok := e.firstRowFrom(p.line); ok {
			return q, true
		}
		return p, false
	case p.row+1 < e.rowCount(p.line):
		return viewPos{p.line, p.row + 1}, true
	}
	if q, ok := e.firstRowFrom(p.line + 1); ok {
		return q, true
	}
	return p, false
}

// firstRowFrom is the first row of the first line from l on that has rows.
func (e *EditorView) firstRowFrom(l int) (viewPos, bool) {
	for ; l < e.buf.Len(); l++ {
		if e.rowCount(l) > 0 {
			return viewPos{l, 0}, true
		}
	}
	return viewPos{}, false
}

// prev is the display row before p; false at the margin row. With virtual,
// rows above the margin exist on line 0. Lines without rows are skipped.
func (e *EditorView) prev(p viewPos, virtual bool) (viewPos, bool) {
	switch {
	case p.line == 0 && p.row <= -1:
		return viewPos{0, p.row - 1}, virtual
	case p.row > 0:
		return viewPos{p.line, p.row - 1}, true
	}
	for l := p.line - 1; l >= 0; l-- {
		if n := e.rowCount(l); n > 0 {
			return viewPos{l, n - 1}, true
		}
	}
	return viewPos{0, -1}, true
}

// cursorRow is the display row holding the cursor and the screen column it
// sits at. The cursor is always on a raw row, below its line's Lead rows.
func (e *EditorView) cursorRow() (viewPos, int) {
	cur := e.buf.Cursor()
	c := e.line(cur.Line)
	row, x := e.geo.CellX(c.text, c.info, c.rows, cur.Col)
	return viewPos{cur.Line, e.leadRows(cur.Line) + row}, x
}

// normalizeTop keeps top on a row that exists after the buffer or the width
// changed under it.
func (e *EditorView) normalizeTop() {
	t := &e.top
	if t.line >= e.buf.Len() {
		*t = viewPos{e.buf.Len() - 1, 0}
	}
	if t.row < 0 {
		return
	}
	if n := e.rowCount(t.line); n > 0 {
		t.row = min(t.row, n-1)
	} else if q, ok := e.firstRowFrom(t.line); ok {
		*t = q
	} else if q, ok := e.prev(viewPos{t.line, 0}, false); ok {
		*t = q
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
	if !e.source {
		e.placeLive(at)
		return
	}
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
	if !e.source {
		return e.exitAnchorLive()
	}
	cur, _ := e.cursorRow()
	return Anchor{Line: cur.line, RowInLine: cur.row, ScreenRow: e.cursorScreenRow()}
}

// moveRows moves the cursor n display rows up (dir < 0) or down at the
// remembered screen column. A single step off the first or last row goes to
// the buffer start or end; a longer move (a page) clamps to that row. With
// extend the move grows the selection.
func (e *EditorView) moveRows(dir, n int, extend bool) {
	if !e.source {
		e.moveRowsLive(dir, n, extend)
		return
	}
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
			e.buf.MoveTo(buffer.Pos{}, extend)
		} else {
			e.buf.MoveTo(e.buf.End(), extend)
		}
		return
	}
	c := e.line(p.line)
	e.buf.MoveTo(buffer.Pos{Line: p.line, Col: e.geo.OffsetAt(c.text, c.info, c.rows, p.row, e.goalX)}, extend)
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
			out[y] = e.drawDisplayRow(p)
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

// selectionStyle marks selected cells. Reverse is an attribute, so it stays
// visible under NO_COLOR.
var selectionStyle = uv.Style{Attrs: uv.AttrReverse}

// drawRow draws one display row with row.Col's leading padding. Selected
// bytes are drawn reversed; the row that ends a selected line adds one
// reversed cell for the line break.
func (e *EditorView) drawRow(p viewPos) string {
	c := e.line(p.line)
	if !c.painted {
		c.spans = e.painter.Paint(e.buf, p.line, c.info, e.scanner)
		c.painted = true
		e.stats.painted++
	}
	r := c.rows[p.row]
	spans, curMatch := e.findOverlay(c.spans, p.line)
	lineBreak := false
	if sel, ok := e.buf.Selection(); ok && sel.Start.Line <= p.line && p.line <= sel.End.Line {
		from, to := 0, len(c.text)
		if p.line == sel.Start.Line {
			from = sel.Start.Col
		}
		if p.line == sel.End.Line {
			to = sel.End.Col
		}
		spans = render.Overlay(spans, from, to, selectionStyle)
		lineBreak = p.line < sel.End.Line && p.row == len(c.rows)-1
	}
	if curMatch != nil {
		spans = render.Overlay(spans, curMatch.Start.Col, curMatch.End.Col, currentMatchStyle)
	}
	s := render.DrawRow(c.text, c.info, r, spans)
	if lineBreak {
		s += selectionStyle.Styled(" ")
	}
	if s == "" {
		return ""
	}
	return strings.Repeat(" ", r.Col) + s
}

// View renders the text window, the completion strip, the rule row and the
// status line.
func (e *EditorView) View() string {
	e.findSync()
	e.syncReveal()
	e.ensureVisible()
	v := strings.Join(e.frameRows(), "\n")
	pad := e.geo.Margin
	if strip := e.completer.View(max(1, e.width-pad)); strip != "" {
		v += "\n" + indentBlock(strip, pad)
	}
	v += "\n" + ruleRow(e.width)
	if e.find != nil {
		bar, _ := e.find.barView(max(1, e.width-pad))
		v += "\n" + indentBlock(bar, pad)
	}
	if p := e.prompt; p != nil {
		row, _ := p.promptView(max(1, e.width-pad))
		v += "\n" + indentBlock(row, pad)
	}
	return v + "\n" + indentBlock(e.statusLine(), pad)
}

// Cursor is where the terminal cursor goes, or nil while a prompt owns the keys.
func (e *EditorView) Cursor() *tea.Cursor {
	if e.mode != editing {
		return nil
	}
	e.findSync()
	e.syncReveal()
	e.ensureVisible()
	if f := e.find; f != nil {
		_, x := f.barView(max(1, e.width-e.geo.Margin))
		return tea.NewCursor(min(e.geo.Margin+x, max(0, e.width-1)), e.textHeight()+e.completer.rows()+1)
	}
	if p := e.prompt; p != nil {
		_, x := p.promptView(max(1, e.width-e.geo.Margin))
		return tea.NewCursor(min(e.geo.Margin+x, max(0, e.width-1)), e.textHeight()+e.completer.rows()+1)
	}
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
