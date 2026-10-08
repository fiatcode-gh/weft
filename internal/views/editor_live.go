package views

import (
	"slices"
	"sort"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// lineSpan is the buffer lines [from, to).
type lineSpan struct{ from, to int }

// previewRow is what the preview gave for one line: its rows, or ok == false
// when the line's chunk failed to render and the line is drawn raw.
type previewRow struct {
	rows render.LineRows
	ok   bool
}

// isShown reports whether line i is in the reveal set.
func (e *EditorView) isShown(i int) bool {
	k := sort.Search(len(e.shown), func(k int) bool { return e.shown[k].to > i })
	return k < len(e.shown) && e.shown[k].from <= i
}

// liveRows is how live preview draws line i: its preview rows, and whether its
// Body is the raw rows (the line is revealed, or its chunk failed to render).
func (e *EditorView) liveRows(i int) (lr render.LineRows, raw bool) {
	e.syncBuffer()
	pr, ok := e.previewed[i]
	if !ok {
		pr.rows, pr.ok = e.preview.Rows(e.buf, e.scanner, i)
		e.previewed[i] = pr
	}
	return pr.rows, !pr.ok || e.isShown(i)
}

// rowCount is the number of display rows of line i: 0 for a line the preview
// draws as nothing.
func (e *EditorView) rowCount(i int) int {
	if e.source {
		return len(e.line(i).rows)
	}
	lr, raw := e.liveRows(i)
	body := len(lr.Body)
	if raw {
		body = len(e.line(i).rows)
	}
	return len(lr.Lead) + body + len(lr.Trail)
}

// leadRows is the number of rows above line i's own rows.
func (e *EditorView) leadRows(i int) int {
	if e.source {
		return 0
	}
	lr, _ := e.liveRows(i)
	return len(lr.Lead)
}

// drawDisplayRow draws display row p: a preview row verbatim, or a raw row
// with unit 3's painter.
func (e *EditorView) drawDisplayRow(p viewPos) string {
	if e.source {
		return e.drawRow(p)
	}
	lr, raw := e.liveRows(p.line)
	r := p.row
	if r < len(lr.Lead) {
		return lr.Lead[r]
	}
	r -= len(lr.Lead)
	body := len(lr.Body)
	if raw {
		body = len(e.line(p.line).rows)
	}
	switch {
	case r < body && raw:
		return e.drawRow(viewPos{p.line, r})
	case r < body:
		return e.findRestyle(p.line, lr.Body[r])
	}
	if r -= body; r < len(lr.Trail) {
		return lr.Trail[r]
	}
	return ""
}

// findRestyle draws every occurrence of the find query in a rendered Body row of
// line with matchStyle, when the open bar has a match on that line. Other rows
// come back unchanged.
func (e *EditorView) findRestyle(line int, row string) string {
	f := e.find
	if f == nil || f.version != e.buf.Version() {
		return row
	}
	i := sort.Search(len(f.matches), func(i int) bool { return f.matches[i].Start.Line >= line })
	if i == len(f.matches) || f.matches[i].Start.Line != line {
		return row
	}
	return render.RestyleRow(row, func(p string) [][2]int { return buffer.MatchIn(p, f.query) }, matchStyle)
}

// revealSpans is the reveal set for the cursor and selection: the unit of the
// cursor's line and, with a selection, everything from the unit of its first
// line to the unit of its last. Sorted and merged.
func (e *EditorView) revealSpans() []lineSpan {
	e.syncBuffer()
	from, to := e.preview.Unit(e.buf, e.scanner, e.buf.Cursor().Line)
	spans := []lineSpan{{from, to}}
	if sel, ok := e.buf.Selection(); ok {
		a, _ := e.preview.Unit(e.buf, e.scanner, sel.Start.Line)
		_, z := e.preview.Unit(e.buf, e.scanner, sel.End.Line)
		spans = append(spans, lineSpan{a, z})
	}
	slices.SortFunc(spans, func(x, y lineSpan) int { return x.from - y.from })
	merged := spans[:1]
	for _, s := range spans[1:] {
		if last := &merged[len(merged)-1]; s.from <= last.to {
			last.to = max(last.to, s.to)
		} else {
			merged = append(merged, s)
		}
	}
	return merged
}

// syncReveal brings the reveal set up to date with the cursor and selection
// and keeps the cursor's line on the screen row the move brought it to: the
// text above shifts to absorb any change in row count. It is the one place
// that decides reveals and pins.
func (e *EditorView) syncReveal() {
	if e.source {
		return
	}
	spans := e.revealSpans()
	cur := e.buf.Cursor()
	if slices.Equal(spans, e.shown) {
		e.revealCur = cur
		return
	}
	pin, found := e.pinRow()
	e.shown, e.revealCur = spans, cur
	if found {
		e.scrollCursorTo(pin)
	} else {
		e.ensureVisible()
	}
}

// pinRow is the screen row, in the layout before the reveal set changes, that
// the cursor's line keeps: its raw row if it was already revealed, else its
// first Body row (its last after a backward move), or, for a line without
// Body rows, where the next line that has rows starts.
func (e *EditorView) pinRow() (int, bool) {
	cur := e.buf.Cursor()
	lr, raw := e.liveRows(cur.Line)
	var want viewPos
	after := false
	switch {
	case raw:
		want, _ = e.cursorRow()
	case len(lr.Body) > 0:
		k := 0
		if cur.Less(e.revealCur) {
			k = len(lr.Body) - 1
		}
		want = viewPos{cur.Line, len(lr.Lead) + k}
	default:
		after = true
	}
	e.normalizeTop()
	p := e.top
	for y := range e.textHeight() {
		if (after && p.line > cur.Line) || (!after && p == want) {
			return y, true
		}
		n, ok := e.next(p)
		if !ok {
			break
		}
		p = n
	}
	return 0, false
}

// rawRow is the index of the cursor's row within its line's raw rows, and the
// screen column.
func (e *EditorView) rawRow() (row, x int) {
	cur := e.buf.Cursor()
	c := e.line(cur.Line)
	return e.geo.CellX(c.text, c.info, c.rows, cur.Col)
}

// moveRowsLive is moveRows in live preview: a step by one goes through the
// cursor line's raw rows and then to the neighbouring source line, whatever
// it looks like; a page walks display rows of the current layout.
func (e *EditorView) moveRowsLive(dir, n int, extend bool) {
	cur := e.buf.Cursor()
	c := e.line(cur.Line)
	row, x := e.rawRow()
	if !e.goalOK {
		e.goalX, e.goalOK = x, true
	}
	if n == 1 {
		if r := row + dir; r >= 0 && r < len(c.rows) {
			e.buf.MoveTo(buffer.Pos{Line: cur.Line, Col: e.geo.OffsetAt(c.text, c.info, c.rows, r, e.goalX)}, extend)
			return
		}
		l := cur.Line + dir
		if l < 0 || l >= e.buf.Len() {
			e.buf.Break()
			if dir < 0 {
				e.buf.MoveTo(buffer.Pos{}, extend)
			} else {
				e.buf.MoveTo(e.buf.End(), extend)
			}
			return
		}
		d := e.line(l)
		r := 0
		if dir < 0 {
			r = len(d.rows) - 1
		}
		e.buf.MoveTo(buffer.Pos{Line: l, Col: e.geo.OffsetAt(d.text, d.info, d.rows, r, e.goalX)}, extend)
		return
	}
	p, _ := e.cursorRow()
	for range n {
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
	}
	d := e.line(p.line)
	e.buf.MoveTo(buffer.Pos{Line: p.line, Col: e.geo.OffsetAt(d.text, d.info, d.rows, 0, e.goalX)}, extend)
}

// placeLive is place in live preview: the anchor's unit is revealed first, then
// the cursor's row goes on the anchor's screen row.
func (e *EditorView) placeLive(at Anchor) {
	line := clampInt(at.Line, 0, e.buf.Len()-1)
	c := e.line(line)
	k := clampInt(at.RowInLine, 0, len(c.rows)-1)
	e.buf.MoveTo(buffer.Pos{Line: line, Col: c.rows[k].Start}, false)
	e.shown, e.revealCur = e.revealSpans(), e.buf.Cursor()
	t, _ := e.cursorRow()
	for range clampInt(at.ScreenRow, 0, e.textHeight()-1) {
		q, _ := e.prev(t, true)
		t = q
	}
	e.top = t
}

// exitAnchorLive is ExitAnchor in live preview: the row index counts raw rows.
func (e *EditorView) exitAnchorLive() Anchor {
	row, _ := e.rawRow()
	return Anchor{Line: e.buf.Cursor().Line, RowInLine: row, ScreenRow: e.cursorScreenRow()}
}
