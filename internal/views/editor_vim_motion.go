package views

import "github.com/fiatcode-gh/weft/v2/internal/buffer"

// motionKind is how an operator reads a motion's range (LD8).
type motionKind int

const (
	motionExclusive motionKind = iota
	motionInclusive
	motionLinewise
)

// vimMotion evaluates motion c from the cursor. forOp selects the operator
// forms: logical j/k and l reaching the line end here; task 08 adds the
// w/W and cw special cases. j, k, ctrl+d and ctrl+u are display-row moves
// that vimRun makes itself, so without forOp they have no target here.
func (e *EditorView) vimMotion(c vimCmd, forOp bool) (target buffer.Pos, kind motionKind, ok bool) {
	b := e.buf
	cur := b.Cursor()
	n := max(c.count, 1)
	switch c.key {
	case "h":
		q, got := b.StepLeft(cur, n)
		return q, motionExclusive, got > 0
	case "l":
		q, got := b.StepRight(cur, n)
		return q, motionExclusive, got > 0
	case "w", "W":
		q := b.WordStart(cur, n, c.key == "W")
		return q, motionExclusive, q != cur
	case "b", "B":
		q := b.WordBack(cur, n, c.key == "B")
		return q, motionExclusive, q != cur
	case "e", "E":
		q := b.WordEnd(cur, n, c.key == "E")
		return q, motionInclusive, q != cur
	case "0":
		return b.LineStart(cur), motionExclusive, true
	case "^":
		return b.FirstNonBlank(cur), motionExclusive, true
	case "$":
		line := min(cur.Line+n-1, b.LastLine())
		return b.LineEnd(buffer.Pos{Line: line}), motionInclusive, true
	case "j", "k":
		if !forOp {
			return cur, motionLinewise, false
		}
		line := cur.Line + n
		if c.key == "k" {
			line = cur.Line - n
		}
		line = max(0, min(line, b.LastLine()))
		return buffer.Pos{Line: line}, motionLinewise, line != cur.Line
	case "gg", "G":
		line := b.LastLine()
		if c.key == "gg" {
			line = 0
		}
		if c.count > 0 {
			line = c.count - 1
		}
		line = max(0, min(line, b.LastLine()))
		return b.FirstNonBlank(buffer.Pos{Line: line}), motionLinewise, true
	case "f", "F", "t", "T":
		e.vim.lastFind.kind, e.vim.lastFind.char = c.key[0], c.arg
		return e.vimFind(cur, n, c.key[0], c.arg, false)
	case ";", ",":
		f := e.vim.lastFind
		if f.kind == 0 {
			return cur, motionExclusive, false
		}
		kind := f.kind
		if c.key == "," {
			kind = flipFind(kind)
		}
		return e.vimFind(cur, n, kind, f.char, true)
	case "%":
		if c.count == 0 {
			q, ok := b.MatchBracket(cur)
			return q, motionInclusive, ok
		}
		last := b.LastLine() + 1
		line := (min(c.count, 100)*last+99)/100 - 1
		return b.FirstNonBlank(buffer.Pos{Line: max(0, min(line, last-1))}), motionLinewise, true
	case "{":
		q := b.ParagraphBack(cur, n)
		return q, motionExclusive, q != cur
	case "}":
		q := b.ParagraphForward(cur, n)
		return q, motionExclusive, q != cur
	}
	return cur, motionExclusive, false
}

// vimFind runs a find motion of the given kind: f and t are inclusive, F and
// T exclusive.
func (e *EditorView) vimFind(cur buffer.Pos, n int, kind byte, char string, repeat bool) (buffer.Pos, motionKind, bool) {
	q, ok := e.buf.FindInLine(cur, n, char, kind, repeat)
	if kind == 'f' || kind == 't' {
		return q, motionInclusive, ok
	}
	return q, motionExclusive, ok
}

// flipFind is the find kind that searches the other way.
func flipFind(kind byte) byte {
	switch kind {
	case 'f':
		return 'F'
	case 'F':
		return 'f'
	case 't':
		return 'T'
	}
	return 't'
}
