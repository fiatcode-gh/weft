package views

import "github.com/fiatcode-gh/weft/v2/internal/buffer"

// mouseReady reports whether the editor takes the mouse: it is editing, with
// no find bar or date prompt owning the keys. It then clears the notice like a
// key press and brings the layout up to date with the one View draws.
func (e *EditorView) mouseReady() bool {
	if e.mode != editing || e.find != nil || e.prompt != nil || (e.vim != nil && e.vim.ex != nil) {
		return false
	}
	e.notice = ""
	e.syncReveal()
	e.ensureVisible()
	return true
}

// landing is the buffer position of screen column x on display row p. A row k
// rows into a line's Body is raw row k, clamped to the line's raw rows, whether
// the line is drawn raw or rendered (the landing rule of a page move).
func (e *EditorView) landing(p viewPos, x int) buffer.Pos {
	c := e.line(p.line)
	if len(c.rows) == 0 {
		return buffer.Pos{Line: p.line}
	}
	r := clampInt(p.row-e.leadRows(p.line), 0, len(c.rows)-1)
	return buffer.Pos{Line: p.line, Col: e.geo.OffsetAt(c.text, c.info, c.rows, r, x)}
}

// moveTo puts the cursor at pos with no selection and lets the view follow.
func (e *EditorView) moveTo(pos buffer.Pos) {
	e.buf.MoveTo(pos, false)
	e.afterKey(false)
}

// Click puts the cursor on the character under screen cell (x, y) and clears
// the selection. Only text rows take clicks; the rule, the status line and the
// completion strip do nothing. Below the last row it goes to the end of the
// buffer.
func (e *EditorView) Click(x, y int) {
	if !e.mouseReady() || x < 0 || y < 0 || y >= e.textHeight() {
		return
	}
	p, below := e.top, false
	for range y {
		n, ok := e.next(p)
		if !ok {
			below = true
			break
		}
		p = n
	}
	for !below && p.row < 0 { // the margin row: the first row of line 0
		n, ok := e.next(p)
		if !ok {
			below = true
			break
		}
		p = n
	}
	pos := e.buf.End()
	if !below {
		pos = e.landing(p, x)
	}
	e.goalOK = false
	if e.vim != nil {
		if e.vim.mode == vimNormal {
			e.vim.pending = nil
		} else if e.vim.mode != vimInsert {
			e.vim.mode = vimNormal
		}
	}
	e.moveTo(pos)
	if e.vim != nil && e.vim.mode != vimInsert {
		e.vimSettle()
	}
}

// Wheel scrolls the text wheelRows display rows up (dir < 0) or down. The
// window stops at the margin row and when its last row is the buffer's last.
// A cursor the scroll took off screen moves to the window's nearest row at its
// goal column; a cursor still on screen stays where it is.
func (e *EditorView) Wheel(dir int) {
	if !e.mouseReady() {
		return
	}
	for range wheelRows {
		if dir < 0 {
			q, ok := e.prev(e.top, false)
			if !ok {
				break
			}
			e.top = q
			continue
		}
		if _, ok := e.next(e.windowBottom()); !ok {
			break
		}
		e.top, _ = e.next(e.top)
	}
	cur, x := e.cursorRow()
	var target viewPos
	switch {
	case cur.less(e.top):
		target = e.top
		for target.row < 0 {
			n, ok := e.next(target)
			if !ok {
				return
			}
			target = n
		}
	case e.windowBottom().less(cur):
		target = e.windowBottom()
	default:
		return
	}
	if !e.goalOK {
		e.goalX, e.goalOK = x, true
	}
	e.moveTo(e.landing(target, e.goalX))
	if e.vim != nil && e.vim.mode != vimInsert {
		e.vimSettle()
	}
}

// windowBottom is the last display row of the text window.
func (e *EditorView) windowBottom() viewPos {
	p := e.top
	for range e.textHeight() - 1 {
		n, ok := e.next(p)
		if !ok {
			break
		}
		p = n
	}
	return p
}
