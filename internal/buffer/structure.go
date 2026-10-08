package buffer

import (
	"regexp"
	"strings"
)

var (
	// Enter-continuation keeps the narrow "- " rule; the empty-bullet rule is as before.
	editBulletRe    = regexp.MustCompile(`^(\s*)- `)
	editEmptyBullet = regexp.MustCompile(`^\s*-\s*$`)
	editBulletBody  = regexp.MustCompile(`^(\s*)- (.*)$`)
	// bulletLineRe is what block moves and subtrees treat as a bullet.
	bulletLineRe = regexp.MustCompile(`^(\s*)[-*+](\s|$)`)
)

// bulletPrefix reports whether line is a "- " bullet and, if so, returns the
// prefix (indent + "- ") to start a continuation line at the same indent.
func bulletPrefix(line string) (string, bool) {
	m := editBulletRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1] + "- ", true
}

// isEmptyBullet reports whether line is a bullet marker with no content
// (just "-" / "- " at some indent), i.e. an Enter here should end the list.
func isEmptyBullet(line string) bool {
	return editEmptyBullet.MatchString(line)
}

// cycleMarkerLine advances a bullet's workflow marker one step in the cycle
// plain -> TODO -> DONE -> plain, returning the rewritten line and the cursor
// column (a byte offset) shifted by the marker-length change. ok is false when line
// is not a "- " bullet. oldCol is the caller's current cursor column on the line.
func cycleMarkerLine(line string, oldCol int) (string, int, bool) {
	m := editBulletBody.FindStringSubmatch(line)
	if m == nil {
		return "", 0, false
	}
	indent, content := m[1], m[2]
	markerCol := len(indent) + 2 // column just after "- "

	var newContent string
	var delta int
	switch {
	case content == "TODO" || strings.HasPrefix(content, "TODO "):
		newContent = "DONE" + content[len("TODO"):]
		delta = 0
	case content == "DONE" || strings.HasPrefix(content, "DONE "):
		if strings.HasPrefix(content, "DONE ") {
			newContent = content[len("DONE "):]
			delta = -len("DONE ")
		} else {
			newContent = ""
			delta = -len("DONE")
		}
	default:
		newContent = "TODO " + content
		delta = len("TODO ")
	}

	newLine := indent + "- " + newContent
	newCol := oldCol
	if oldCol >= markerCol {
		newCol = max(oldCol+delta, markerCol)
	}
	return newLine, min(newCol, len(newLine)), true
}

func isBlank(line string) bool { return strings.TrimSpace(line) == "" }

func isBullet(line string) bool { return bulletLineRe.MatchString(line) }

// indentCells is the display width of line's leading whitespace, a tab advancing to
// the next multiple of 4.
func indentCells(line string) int {
	w := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			w++
		case '\t':
			w += 4 - w%4
		default:
			return w
		}
	}
	return w
}

// indentLine adds one indent unit to the front of a non-blank line: a tab when the
// line's leading whitespace starts with one, else two spaces. Blank lines are unchanged.
func indentLine(line string) string {
	switch {
	case isBlank(line):
		return line
	case strings.HasPrefix(line, "\t"):
		return "\t" + line
	default:
		return "  " + line
	}
}

// dedentLine removes one leading tab, or up to two leading spaces, returning the new
// line and the number of bytes removed.
func dedentLine(line string) (string, int) {
	if strings.HasPrefix(line, "\t") {
		return line[1:], 1
	}
	n := 0
	for n < 2 && n < len(line) && line[n] == ' ' {
		n++
	}
	return line[n:], n
}

// docLines is the number of lines that can belong to a block: the document's final
// empty line (the one after a final newline) never does.
func (b *Buffer) docLines() int {
	n := len(b.lines)
	if n > 1 && b.lines[n-1] == "" {
		return n - 1
	}
	return n
}

// blockEnd is the end (exclusive) of the block that starts at line i: for a bullet its
// subtree (every later line up to the first non-blank line indented no deeper,
// trailing blank lines included); for any other line, the line itself.
func (b *Buffer) blockEnd(i int) int {
	if !isBullet(b.lines[i]) {
		return i + 1
	}
	c, lim := indentCells(b.lines[i]), b.docLines()
	j := i + 1
	for j < lim && (isBlank(b.lines[j]) || indentCells(b.lines[j]) > c) {
		j++
	}
	return j
}

// Newline is Enter: it replaces the selection; on an empty bullet it clears the line;
// on a "- " bullet it breaks and repeats the indent and "- "; otherwise it breaks.
// The bullet is continued only when the cursor is past its marker.
func (b *Buffer) Newline() {
	b.edit(func() {
		r, sel := b.Selection()
		if sel {
			b.replace(r, "")
			b.cursor, b.hasAnchor = r.Start, false
		}
		l, col := b.cursor.Line, b.cursor.Col
		line := b.lines[l]
		ins := b.eol
		if !sel && isEmptyBullet(line) {
			b.replace(Range{Pos{l, 0}, Pos{l, len(line)}}, "")
			b.cursor = Pos{l, 0}
			return
		}
		if prefix, ok := bulletPrefix(line); ok && col >= len(prefix) {
			ins += prefix
		}
		b.replace(Range{b.cursor, b.cursor}, ins)
		b.cursor = Pos{l + 1, len(ins) - len(b.eol)}
	})
}

// CycleMarker advances the workflow marker of a "- " bullet under the cursor
// (plain → TODO → DONE → plain). It reports false, opening no group, otherwise.
func (b *Buffer) CycleMarker() bool {
	l := b.cursor.Line
	line := b.lines[l]
	newLine, newCol, ok := cycleMarkerLine(line, b.cursor.Col)
	if !ok {
		return false
	}
	b.edit(func() {
		b.replace(Range{Pos{l, 0}, Pos{l, len(line)}}, newLine)
		b.cursor, b.hasAnchor = Pos{l, newCol}, false
	})
	return true
}

// Indent is tab: see shiftLines.
func (b *Buffer) Indent() { b.shiftLines(+1) }

// Outdent is shift+tab: see shiftLines.
func (b *Buffer) Outdent() { b.shiftLines(-1) }

// shiftLines indents (dir > 0) or outdents each line of the target by one unit. The
// target is the selection's lines (a last line whose selection column is 0 excluded),
// else the subtree of a bullet cursor line, else the cursor line. Cursor and selection
// endpoints move with their lines' columns. Nothing changing opens no group.
func (b *Buffer) shiftLines(dir int) {
	first, last := b.cursor.Line, b.cursor.Line
	sel, hasSel := b.Selection()
	switch {
	case hasSel:
		first, last = sel.Start.Line, sel.End.Line
		if sel.End.Col == 0 && last > first {
			last--
		}
	case isBullet(b.lines[first]):
		if dir < 0 && indentCells(b.lines[first]) == 0 {
			return
		}
		last = b.blockEnd(first) - 1
	}

	delta := make(map[int]int) // line -> bytes added (+) or removed (-) at the front
	for l := first; l <= last; l++ {
		var n int
		if dir > 0 {
			n = len(indentLine(b.lines[l])) - len(b.lines[l])
		} else {
			_, removed := dedentLine(b.lines[l])
			n = -removed
		}
		if n != 0 {
			delta[l] = n
		}
	}
	if len(delta) == 0 {
		return
	}

	shift := func(p Pos) Pos {
		p.Col = max(0, p.Col+delta[p.Line])
		return p
	}
	cursor, anchor := shift(b.cursor), shift(b.anchor)
	b.edit(func() {
		for l := first; l <= last; l++ {
			n, ok := delta[l]
			switch {
			case !ok:
			case n > 0:
				b.replace(Range{Pos{l, 0}, Pos{l, 0}}, indentLine(b.lines[l])[:n])
			default:
				b.replace(Range{Pos{l, 0}, Pos{l, -n}}, "")
			}
		}
		b.cursor, b.anchor = cursor, anchor
	})
}

// MoveBlock moves the cursor line's block one sibling up (dir < 0) or down (dir > 0):
// a bullet with its subtree past the neighbouring bullet at the same indent, any other
// line past the adjacent line. It reports false, changing nothing, when there is no
// such neighbour. The cursor stays on the same text; the selection is cleared.
func (b *Buffer) MoveBlock(dir int) bool {
	i := b.cursor.Line
	lim := b.docLines()
	if i >= lim {
		return false
	}
	end := b.blockEnd(i)
	c := indentCells(b.lines[i])
	bullet := isBullet(b.lines[i])

	var from, mid, to int // swap [from,mid) with [mid,to)
	var newLine int
	if dir < 0 {
		k := i - 1
		if bullet {
			for k >= 0 && (isBlank(b.lines[k]) || indentCells(b.lines[k]) > c) {
				k--
			}
			if k < 0 || !isBullet(b.lines[k]) || indentCells(b.lines[k]) != c {
				return false
			}
		} else if k < 0 {
			return false
		}
		from, mid, to, newLine = k, i, end, k
	} else {
		if end >= lim || bullet && (!isBullet(b.lines[end]) || indentCells(b.lines[end]) != c) {
			return false
		}
		sibEnd := end + 1
		if bullet {
			sibEnd = b.blockEnd(end)
		}
		from, mid, to, newLine = i, end, sibEnd, i+sibEnd-end
	}

	type item struct{ text, term string }
	items := make([]item, 0, to-from)
	for l := from; l < to; l++ {
		items = append(items, item{b.lines[l], b.Terminator(l)})
	}
	n1 := mid - from
	moved := append(append([]item{}, items[n1:]...), items[:n1]...)
	atEnd := to == len(b.lines)
	var sb strings.Builder
	for k, it := range moved {
		term := it.term
		if atEnd {
			switch {
			case k == len(moved)-1:
				term = ""
			case term == "":
				term = b.eol
			}
		}
		sb.WriteString(it.text)
		sb.WriteString(term)
	}

	col := b.cursor.Col
	rng := Range{Pos{from, 0}, Pos{to, 0}}
	if atEnd {
		rng.End = b.End()
	}
	b.edit(func() {
		b.hasAnchor = false
		b.replace(rng, sb.String())
		b.cursor = b.clampPos(Pos{newLine, col})
	})
	return true
}
