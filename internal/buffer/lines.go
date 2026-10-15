package buffer

import "strings"

// Whole-line text and the line and put edits of the modal mode.

// lineSpan is the range that removes lines first..last with one line break:
// the break after them, else the break before them, else everything.
func (b *Buffer) lineSpan(first, last int) Range {
	switch {
	case last < len(b.lines)-1:
		return Range{Pos{first, 0}, Pos{last + 1, 0}}
	case first > 0:
		return Range{Pos{first - 1, len(b.lines[first-1])}, Pos{last, len(b.lines[last])}}
	default:
		return Range{Pos{0, 0}, Pos{last, len(b.lines[last])}}
	}
}

// lineArgs clamps first and last to [0, LastLine()] and orders them.
func (b *Buffer) lineArgs(first, last int) (int, int) {
	top := b.LastLine()
	first, last = min(max(first, 0), top), min(max(last, 0), top)
	if last < first {
		first, last = last, first
	}
	return first, last
}

// leadingBlanks is the run of spaces and tabs at the start of line.
func leadingBlanks(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// LineText is lines first..last joined by "\n", plus "\n": a linewise copy.
func (b *Buffer) LineText(first, last int) string {
	first, last = b.lineArgs(first, last)
	var sb strings.Builder
	for i := first; i <= last; i++ {
		sb.WriteString(b.lines[i])
		sb.WriteByte('\n')
	}
	return sb.String()
}

// DeleteLines removes lines first..last with their line break and puts the
// cursor on FirstNonBlank of the line now at first (clamped to LastLine).
func (b *Buffer) DeleteLines(first, last int) {
	first, last = b.lineArgs(first, last)
	b.edit(func() {
		b.replace(b.lineSpan(first, last), "")
		b.cursor, b.hasAnchor = Pos{min(first, b.LastLine()), 0}, false
		b.cursor = b.FirstNonBlank(b.cursor)
	})
}

// ChangeLines replaces the text of lines first..last with first's leading
// blanks, leaving one line, the cursor at its end.
func (b *Buffer) ChangeLines(first, last int) {
	first, last = b.lineArgs(first, last)
	indent := leadingBlanks(b.lines[first])
	b.edit(func() {
		b.replace(Range{Pos{first, 0}, Pos{last, len(b.lines[last])}}, indent)
		b.cursor, b.hasAnchor = Pos{first, len(indent)}, false
	})
}

// OpenLine adds a line below (above when above) the cursor line holding the
// cursor line's "- " bullet prefix, else its leading blanks; cursor at its end.
func (b *Buffer) OpenLine(above bool) {
	l := b.cursor.Line
	prefix, ok := bulletPrefix(b.lines[l])
	if !ok {
		prefix = leadingBlanks(b.lines[l])
	}
	b.edit(func() {
		if above {
			b.replace(Range{Pos{l, 0}, Pos{l, 0}}, prefix+b.eol)
			b.cursor = Pos{l, len(prefix)}
		} else {
			at := b.lineEndPos(l)
			b.replace(Range{at, at}, b.eol+prefix)
			b.cursor = Pos{l + 1, len(prefix)}
		}
		b.hasAnchor = false
	})
}

// Put inserts text n times: linewise below (after) or above the cursor line,
// charwise after or at the cursor. The cursor lands on the first non-blank of
// the first inserted line (linewise), on the start of the last inserted
// grapheme (charwise, single line) or at the insertion point (multi-line).
func (b *Buffer) Put(text string, linewise, after bool, n int) {
	if text == "" {
		return
	}
	n = max(n, 1)
	if linewise && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	multi := strings.Contains(text, "\n")
	ins := strings.Repeat(text, n)
	l := b.cursor.Line
	b.edit(func() {
		switch {
		case linewise && after && l < len(b.lines)-1:
			b.replace(Range{Pos{l + 1, 0}, Pos{l + 1, 0}}, b.toRaw(ins))
			b.cursor = b.FirstNonBlank(Pos{l + 1, 0})
		case linewise && after:
			at := b.lineEndPos(l)
			b.replace(Range{at, at}, b.eol+b.toRaw(strings.TrimSuffix(ins, "\n")))
			b.cursor = b.FirstNonBlank(Pos{l + 1, 0})
		case linewise:
			b.replace(Range{Pos{l, 0}, Pos{l, 0}}, b.toRaw(ins))
			b.cursor = b.FirstNonBlank(Pos{l, 0})
		default:
			at := b.cursor
			if after {
				at, _ = b.StepRight(at, 1)
			}
			raw := b.toRaw(ins)
			b.replace(Range{at, at}, raw)
			end := endPos(at, raw)
			if multi {
				b.cursor = at
			} else {
				b.cursor = Pos{end.Line, prevStart(b.lines[end.Line], end.Col)}
			}
		}
		b.hasAnchor = false
	})
}
