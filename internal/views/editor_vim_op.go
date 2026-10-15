package views

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

// vimShortcuts are the one-key commands that are an operator and a motion
// (LD10); they keep the typed count.
var vimShortcuts = map[string][2]string{
	"x": {"d", "l"}, "X": {"d", "h"}, "s": {"c", "l"},
	"S": {"c", "c"}, "D": {"d", "$"}, "C": {"c", "$"},
}

// vimSpan is the text an operator acts on: the half-open range r, or the
// lines first..last when linewise.
type vimSpan struct {
	r           buffer.Range
	first, last int
	linewise    bool
}

// vimLines is the linewise span of lines a..b in either order.
func vimLines(a, b int) vimSpan {
	return vimSpan{first: min(a, b), last: max(a, b), linewise: true}
}

// vimSpanOf is the span of c's target: the doubled operator's lines, a text
// object, or the motion from the cursor. false when there is none.
func (e *EditorView) vimSpanOf(c vimCmd) (vimSpan, bool) {
	b := e.buf
	cur := b.Cursor()
	n := max(c.count, 1)
	switch {
	case c.key == c.op:
		return vimLines(cur.Line, min(cur.Line+n-1, b.LastLine())), true
	case len(c.key) == 2 && (c.key[0] == 'i' || c.key[0] == 'a'):
		return e.vimObjectSpan(cur, n, c.key[0] == 'a', c.key[1])
	}
	q, kind, ok := e.vimMotion(c, true)
	if !ok {
		return vimSpan{}, false
	}
	if kind == motionLinewise {
		return vimLines(cur.Line, q.Line), true
	}
	from, to := cur, q
	if to.Less(from) {
		from, to = to, from
	}
	if kind == motionInclusive {
		to, _ = b.StepRight(to, 1)
	}
	return vimSpan{r: buffer.Range{Start: from, End: to}}, true
}

// vimObjectSpan is the text object obj (the character after i or a, a
// bracket named by its opener) around the cursor.
func (e *EditorView) vimObjectSpan(cur buffer.Pos, n int, around bool, obj byte) (vimSpan, bool) {
	b := e.buf
	var r buffer.Range
	var ok bool
	switch obj {
	case 'w', 'W':
		r, ok = b.WordObject(cur, n, around, obj == 'W')
	case '"', '\'':
		r, ok = b.QuoteObject(cur, obj, around)
	case '(', '[', '{':
		r, ok = b.BracketObject(cur, n, obj, around)
	case 'p':
		first, last, found := b.ParagraphObject(cur, n, around)
		return vimLines(first, last), found
	}
	return vimSpan{r: r}, ok
}

// vimOperate applies operator c.op to the span of c's motion or object. A
// target that does not exist drops the operator. Every change but c is one
// undo step; c leaves its step open for the insert session.
func (e *EditorView) vimOperate(c vimCmd) tea.Cmd {
	s, ok := e.vimSpanOf(c)
	if !ok {
		return nil
	}
	b := e.buf
	mark := b.UndoMark()
	var cmd tea.Cmd
	switch c.op {
	case "c":
		return e.vimChange(s, mark)
	case "d":
		cmd = e.vimDelete(s)
	case "y":
		cmd = e.vimYank(s)
	default:
		e.vimShift(c, s)
	}
	b.Squash(mark)
	e.goalOK = false
	return cmd
}

// vimCopy puts the span's text in the register and the clipboard.
func (e *EditorView) vimCopy(s vimSpan) tea.Cmd {
	if s.linewise {
		return e.toRegister(e.buf.LineText(s.first, s.last), true, "")
	}
	return e.toRegister(e.buf.Text(s.r), false, "")
}

// empty reports a characterwise span with no text.
func (s vimSpan) empty() bool { return !s.linewise && s.r.Start == s.r.End }

func (e *EditorView) vimDelete(s vimSpan) tea.Cmd {
	if s.empty() {
		return nil
	}
	cmd := e.vimCopy(s)
	if s.linewise {
		e.buf.DeleteLines(s.first, s.last)
	} else {
		e.buf.DeleteRange(s.r)
	}
	return cmd
}

func (e *EditorView) vimChange(s vimSpan, mark int) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case s.linewise:
		cmd = e.vimCopy(s)
		e.buf.ChangeLines(s.first, s.last)
	default:
		if !s.empty() {
			cmd = e.vimCopy(s)
			e.buf.DeleteRange(s.r)
		}
		e.buf.MoveTo(s.r.Start, false)
	}
	e.vimEnterInsert(mark)
	return cmd
}

// vimYank copies the span; the cursor goes to the start of a characterwise
// span and to the first line of a linewise span above it.
func (e *EditorView) vimYank(s vimSpan) tea.Cmd {
	if s.empty() {
		return nil
	}
	cmd := e.vimCopy(s)
	cur := e.buf.Cursor()
	switch {
	case !s.linewise:
		e.buf.MoveTo(s.r.Start, false)
	case s.first < cur.Line:
		e.buf.MoveTo(buffer.Pos{Line: s.first, Col: cur.Col}, false)
	}
	return cmd
}

// vimShift shifts the lines the span touches one level, the cursor on the
// first non-blank of the first. A doubled operator on one line is Tab or
// Shift+Tab, which move a bullet with its children.
func (e *EditorView) vimShift(c vimCmd, s vimSpan) {
	b := e.buf
	dir := 1
	if c.op == "<" {
		dir = -1
	}
	first, last := s.first, s.last
	if !s.linewise {
		first, last = s.r.Start.Line, s.r.End.Line
		if s.r.End.Col == 0 && last > first {
			last--
		}
	}
	switch {
	case c.key == c.op && max(c.count, 1) == 1 && dir > 0:
		b.Indent()
	case c.key == c.op && max(c.count, 1) == 1:
		b.Outdent()
	default:
		b.ShiftLines(first, last, dir)
	}
	b.MoveTo(b.FirstNonBlank(buffer.Pos{Line: first}), false)
}

// vimReplace is r{c}: the next count graphemes become that character. It
// fails when fewer remain on the line.
func (e *EditorView) vimReplace(c vimCmd) {
	b := e.buf
	n := max(c.count, 1)
	cur := b.Cursor()
	end, got := b.StepRight(cur, n)
	if got < n {
		return
	}
	mark := b.UndoMark()
	b.ReplaceRange(buffer.Range{Start: cur, End: end}, strings.Repeat(c.arg, n))
	last, _ := b.StepLeft(b.Cursor(), 1)
	b.MoveTo(last, false)
	b.Squash(mark)
	e.goalOK = false
}

// vimJoin is J: join count lines (at least two) into the cursor line.
func (e *EditorView) vimJoin(c vimCmd) {
	b := e.buf
	mark := b.UndoMark()
	if b.JoinLines(b.Cursor().Line, c.count) {
		b.Squash(mark)
		e.goalOK = false
	}
}

// vimPut is p and P: put the register count times.
func (e *EditorView) vimPut(c vimCmd) {
	if e.reg.text == "" {
		e.notice = "nothing copied yet"
		return
	}
	b := e.buf
	mark := b.UndoMark()
	b.Put(e.reg.text, e.reg.linewise, c.key == "p", c.count)
	b.Squash(mark)
	e.goalOK = false
}
