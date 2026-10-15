package views

import (
	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v3/internal/buffer"
)

// vimExtent is the size of a visual selection, which `.` applies again from
// the cursor (LD13): whole lines, graphemes on one line, or lines up to a
// grapheme column on the last.
type vimExtent struct {
	linewise    bool
	lines, cols int
}

// vimVisualOps are the visual-mode keys that change the buffer or fill the
// register.
var vimVisualOps = map[string]bool{
	"d": true, "x": true, "c": true, "y": true, ">": true, "<": true,
	"J": true, "~": true, "u": true, "U": true,
}

// vimVisual reports whether the editor is in visual or visual line mode.
func (e *EditorView) vimVisual() bool {
	return e.vim != nil && (e.vim.mode == vimVisual || e.vim.mode == vimVisualLine)
}

// selection is the range the frame draws and the reveal set covers: the
// visual range in visual modes (LD13), else the buffer's own selection.
func (e *EditorView) selection() (buffer.Range, bool) {
	if !e.vimVisual() {
		return e.buf.Selection()
	}
	b := e.buf
	lo, hi := e.visualEnds()
	if e.vim.mode == vimVisualLine {
		return buffer.Range{
			Start: buffer.Pos{Line: lo.Line},
			End:   buffer.Pos{Line: hi.Line, Col: len(b.Line(hi.Line))},
		}, true
	}
	return buffer.Range{Start: lo, End: b.Right(hi)}, true
}

// visualEnds are the anchor and the cursor in text order.
func (e *EditorView) visualEnds() (lo, hi buffer.Pos) {
	lo, hi = e.vim.anchor, e.buf.Cursor()
	if hi.Less(lo) {
		lo, hi = hi, lo
	}
	return lo, hi
}

// vimStartVisual begins visual mode of the given kind at the cursor.
func (e *EditorView) vimStartVisual(mode vimMode) {
	e.vim.mode = mode
	e.vim.anchor = e.buf.Cursor()
}

// vimVisualKey handles one key in visual and visual line mode: Esc leaves,
// Ctrl+S saves, every other chord does nothing.
func (e *EditorView) vimVisualKey(msg tea.KeyPressMsg) (EditorResult, tea.Cmd) {
	v := e.vim
	switch msg.String() {
	case keyEsc:
		v.pending = nil
		v.mode = vimNormal
		e.vimSettle()
		return EditorResult{}, nil
	case "ctrl+s":
		v.pending = nil
		res, cmd := e.editKey(msg)
		if e.mode == editing && e.prompt == nil && e.find == nil {
			e.vimSettle()
		}
		return res, cmd
	}
	tok := vimToken(msg)
	if alias, ok := vimAlias[tok]; ok {
		tok = alias
	}
	v.pending = append(v.pending, tok)
	var cmd tea.Cmd
	switch c, st := parseVim(v.pending, true); st {
	case vimDone:
		v.pending = nil
		cmd = e.vimVisualRun(c)
	case vimBad:
		v.pending = nil
	}
	e.vimSettle()
	return EditorResult{}, cmd
}

// vimVisualRun executes one parsed visual-mode command: a motion, `o`, `v`,
// `V` or an operator on the selection.
func (e *EditorView) vimVisualRun(c vimCmd) tea.Cmd {
	v := e.vim
	switch {
	case c.key == "o":
		cur := e.buf.Cursor()
		e.buf.MoveTo(v.anchor, false)
		v.anchor = cur
		e.goalOK = false
	case c.key == "v" && v.mode == vimVisual, c.key == "V" && v.mode == vimVisualLine:
		v.mode = vimNormal
	case c.key == "v":
		v.mode = vimVisual
	case c.key == "V":
		v.mode = vimVisualLine
	case vimVisualOps[c.key]:
		return e.vimVisualOperate(c)
	default:
		e.vimExec(c)
	}
	return nil
}

// vimVisualOperate applies operator c.key to the selection, returns to
// normal mode (or insert for c) and records the change for `.` with the
// selection's extent.
func (e *EditorView) vimVisualOperate(c vimCmd) tea.Cmd {
	v := e.vim
	r, _ := e.selection()
	linewise := v.mode == vimVisualLine
	ext := e.visualExtent()
	v.mode = vimNormal
	before := e.buf.Version()
	cmd := e.vimVisualOp(c.key, c.count, r, linewise)
	e.goalOK = false
	rec := &vimChange{cmd: c, extent: &ext}
	switch {
	case v.mode == vimInsert:
		v.insCount = 1
		v.rec = rec
	case c.key != "y" && e.buf.Version() != before:
		v.last = rec
	}
	return cmd
}

// vimVisualOp applies operator op to the range r, whole lines when linewise.
// Every operator but c is one undo step; c leaves its step open for the
// insert session.
func (e *EditorView) vimVisualOp(op string, count int, r buffer.Range, linewise bool) tea.Cmd {
	b := e.buf
	first, last := r.Start.Line, r.End.Line
	if !linewise && r.End.Col == 0 && last > first {
		last--
	}
	s := vimSpan{r: r, first: first, last: last, linewise: linewise}
	mark := b.UndoMark()
	var cmd tea.Cmd
	switch op {
	case "c":
		return e.vimChange(s, mark)
	case "d", "x":
		cmd = e.vimDelete(s)
	case "y":
		if !s.empty() {
			cmd = e.vimCopy(s)
		}
		b.MoveTo(r.Start, false)
	case ">", "<":
		dir := 1
		if op == "<" {
			dir = -1
		}
		for range max(count, 1) {
			b.ShiftLines(first, last, dir)
		}
		b.MoveTo(b.FirstNonBlank(buffer.Pos{Line: first}), false)
	case "J":
		b.JoinLines(first, max(2, last-first+1))
	default:
		mode := map[string]buffer.CaseMode{"~": buffer.CaseToggle, "u": buffer.CaseLower, "U": buffer.CaseUpper}[op]
		b.CaseRange(r, mode)
	}
	b.Squash(mark)
	return cmd
}

// graphemesBetween counts the graphemes from a to b, both on one line.
func (e *EditorView) graphemesBetween(a, z buffer.Pos) int {
	n := 0
	for p := a; p.Col < z.Col; n++ {
		q, got := e.buf.StepRight(p, 1)
		if got == 0 {
			break
		}
		p = q
	}
	return n
}

// visualExtent is the size of the current selection (LD13).
func (e *EditorView) visualExtent() vimExtent {
	lo, hi := e.visualEnds()
	lines := hi.Line - lo.Line + 1
	switch {
	case e.vim.mode == vimVisualLine:
		return vimExtent{linewise: true, lines: lines}
	case lines == 1:
		end, _ := e.buf.StepRight(hi, 1)
		return vimExtent{lines: 1, cols: e.graphemesBetween(lo, end)}
	}
	return vimExtent{lines: lines, cols: e.graphemesBetween(buffer.Pos{Line: hi.Line}, hi)}
}

// vimVisualDot is `.` for a visual change: the range of rec's extent from the
// cursor, through the same operator.
func (e *EditorView) vimVisualDot(rec *vimChange) tea.Cmd {
	b := e.buf
	cur := b.Cursor()
	x := rec.extent
	var r buffer.Range
	switch {
	case x.linewise:
		last := min(cur.Line+x.lines-1, b.LastLine())
		r = buffer.Range{
			Start: buffer.Pos{Line: cur.Line},
			End:   buffer.Pos{Line: last, Col: len(b.Line(last))},
		}
	case x.lines == 1:
		end := cur
		if x.cols > 1 {
			end, _ = b.StepRight(cur, x.cols-1)
		}
		r = buffer.Range{Start: cur, End: b.Right(end)}
	default:
		line := min(cur.Line+x.lines-1, b.LastLine())
		end := buffer.Pos{Line: line}
		if x.cols > 0 {
			end, _ = b.StepRight(end, x.cols)
		}
		if lastCol := b.LineLast(end); end.Col > lastCol.Col {
			end = lastCol
		}
		r = buffer.Range{Start: cur, End: b.Right(end)}
	}
	return e.vimVisualOp(rec.cmd.key, rec.cmd.count, r, x.linewise)
}
