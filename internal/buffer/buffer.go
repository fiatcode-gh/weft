// Package buffer is the pure editing model behind weft's in-app editor: byte-faithful
// line storage, cursor and selection, grapheme and word movement, and undo/redo with
// typing groups. It has no UI imports.
package buffer

import (
	"slices"
	"strings"
)

// Pos is a position in the buffer. Col is a byte offset into Line(Line).
type Pos struct{ Line, Col int }

// Less reports whether p comes before q.
func (p Pos) Less(q Pos) bool {
	if p.Line != q.Line {
		return p.Line < q.Line
	}
	return p.Col < q.Col
}

// Range is a span of text, Start <= End.
type Range struct{ Start, End Pos }

// Buffer stores text as lines without terminators plus a per-line CRLF flag, so that
// String reproduces the loaded bytes exactly. A file ending in a newline has a final
// empty line; the last line never has a terminator.
type Buffer struct {
	lines []string
	crlf  []bool // crlf[i]: line i ends with "\r\n" (always false for the last line)
	eol   string

	cursor    Pos
	anchor    Pos
	hasAnchor bool

	version     int
	changedFrom int
	changed     bool

	undo, redo []*group
	pending    *group // group being built (depth > 0)
	depth      int
	last       *group // top of the undo stack, the only group coalescing may extend
	kind       coalesceKind
	next       Pos // where the next coalescing operation must start
}

// New loads content. Lines split on "\n"; a "\r" directly before it marks a CRLF
// terminator; any other "\r" is text.
func New(content string) *Buffer {
	b := &Buffer{}
	b.lines, b.crlf = parseRaw(content)
	lf, cr := 0, 0
	for i := 0; i < len(b.crlf); i++ {
		if b.crlf[i] {
			cr++
		} else if i < len(b.lines)-1 {
			lf++
		}
	}
	b.eol = "\n"
	if cr > lf {
		b.eol = "\r\n"
	}
	return b
}

// parseRaw splits raw text into line texts and CRLF flags. The last element has no
// terminator, so the result always has at least one line.
func parseRaw(raw string) ([]string, []bool) {
	n := strings.Count(raw, "\n") + 1
	texts := make([]string, 0, n)
	flags := make([]bool, 0, n)
	for {
		i := strings.IndexByte(raw, '\n')
		if i < 0 {
			texts = append(texts, raw)
			flags = append(flags, false)
			return texts, flags
		}
		line := raw[:i]
		cr := strings.HasSuffix(line, "\r")
		if cr {
			line = line[:len(line)-1]
		}
		texts = append(texts, line)
		flags = append(flags, cr)
		raw = raw[i+1:]
	}
}

// String returns the exact bytes of the buffer.
func (b *Buffer) String() string {
	n := 0
	for i, l := range b.lines {
		n += len(l) + len(b.Terminator(i))
	}
	var sb strings.Builder
	sb.Grow(n)
	for i, l := range b.lines {
		sb.WriteString(l)
		sb.WriteString(b.Terminator(i))
	}
	return sb.String()
}

// EqualString reports whether the buffer's exact bytes equal s, without allocating.
func (b *Buffer) EqualString(s string) bool {
	for i, l := range b.lines {
		if !strings.HasPrefix(s, l) {
			return false
		}
		s = s[len(l):]
		t := b.Terminator(i)
		if !strings.HasPrefix(s, t) {
			return false
		}
		s = s[len(t):]
	}
	return s == ""
}

// Len is the number of lines (at least 1).
func (b *Buffer) Len() int { return len(b.lines) }

// Line returns line i without its terminator.
func (b *Buffer) Line(i int) string { return b.lines[i] }

// EOL is the terminator for line breaks the user creates: "\n" or "\r\n".
func (b *Buffer) EOL() string { return b.eol }

// End is the position after the last byte.
func (b *Buffer) End() Pos {
	last := len(b.lines) - 1
	return Pos{last, len(b.lines[last])}
}

// Text returns the text in r with line breaks as "\n".
func (b *Buffer) Text(r Range) string {
	r = b.clampRange(r)
	if r.Start.Line == r.End.Line {
		return b.lines[r.Start.Line][r.Start.Col:r.End.Col]
	}
	var sb strings.Builder
	sb.WriteString(b.lines[r.Start.Line][r.Start.Col:])
	for i := r.Start.Line + 1; i < r.End.Line; i++ {
		sb.WriteByte('\n')
		sb.WriteString(b.lines[i])
	}
	sb.WriteByte('\n')
	sb.WriteString(b.lines[r.End.Line][:r.End.Col])
	return sb.String()
}

// Version increases by one per mutation, including undo and redo.
func (b *Buffer) Version() int { return b.version }

// Changed returns the first line touched since the previous call and resets.
func (b *Buffer) Changed() (from int, ok bool) {
	from, ok = b.changedFrom, b.changed
	b.changed = false
	return from, ok
}

// Terminator is the exact terminator of line i ("" for the last line).
func (b *Buffer) Terminator(i int) string {
	switch {
	case i >= len(b.lines)-1:
		return ""
	case b.crlf[i]:
		return "\r\n"
	default:
		return "\n"
	}
}

// chunk is text as the buffer holds it: line texts and their CRLF flags (the last
// line is unterminated). History stores chunks instead of raw bytes: a line "a\r"
// ended by "\n" and a line "a" ended by "\r\n" have the same bytes but are different
// lines, and later history entries address columns of the real ones.
type chunk struct {
	texts []string
	crlf  []bool
}

// parseChunk is parseRaw as a chunk.
func parseChunk(raw string) chunk {
	texts, flags := parseRaw(raw)
	return chunk{texts, flags}
}

// String returns the bytes of c, terminators included.
func (c chunk) String() string {
	var sb strings.Builder
	for i, t := range c.texts {
		sb.WriteString(t)
		if i < len(c.texts)-1 {
			if c.crlf[i] {
				sb.WriteString("\r\n")
			} else {
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}

// end is the position after inserting c at at.
func (c chunk) end(at Pos) Pos {
	last := len(c.texts) - 1
	if last == 0 {
		return Pos{at.Line, at.Col + len(c.texts[0])}
	}
	return Pos{at.Line + last, len(c.texts[last])}
}

// cut returns the exact lines of r, terminators included.
func (b *Buffer) cut(r Range) chunk {
	if r.Start.Line == r.End.Line {
		return chunk{[]string{b.lines[r.Start.Line][r.Start.Col:r.End.Col]}, []bool{false}}
	}
	n := r.End.Line - r.Start.Line + 1
	c := chunk{make([]string, n), make([]bool, n)}
	copy(c.texts, b.lines[r.Start.Line:r.End.Line+1])
	copy(c.crlf, b.crlf[r.Start.Line:r.End.Line])
	c.texts[0] = c.texts[0][r.Start.Col:]
	c.texts[n-1] = c.texts[n-1][:r.End.Col]
	return c
}

func (b *Buffer) clampPos(p Pos) Pos {
	p.Line = min(max(p.Line, 0), len(b.lines)-1)
	p.Col = min(max(p.Col, 0), len(b.lines[p.Line]))
	return p
}

func (b *Buffer) clampRange(r Range) Range {
	r.Start, r.End = b.clampPos(r.Start), b.clampPos(r.End)
	if r.End.Less(r.Start) {
		r.Start, r.End = r.End, r.Start
	}
	return r
}

// endPos is the position after inserting raw at at.
func endPos(at Pos, raw string) Pos {
	n := strings.Count(raw, "\n")
	if n == 0 {
		return Pos{at.Line, at.Col + len(raw)}
	}
	return Pos{at.Line + n, len(raw) - strings.LastIndexByte(raw, '\n') - 1}
}

// splice replaces r with c without recording history. Terminators of c are kept; the
// range's end line keeps its own terminator. c is not modified.
func (b *Buffer) splice(r Range, c chunk) {
	texts, flags := slices.Clone(c.texts), slices.Clone(c.crlf)
	head := b.lines[r.Start.Line][:r.Start.Col]
	tail := b.lines[r.End.Line][r.End.Col:]
	last := len(texts) - 1
	flags[last] = b.crlf[r.End.Line]
	texts[0] = head + texts[0]
	texts[last] += tail
	b.lines = slices.Replace(b.lines, r.Start.Line, r.End.Line+1, texts...)
	b.crlf = slices.Replace(b.crlf, r.Start.Line, r.End.Line+1, flags...)
	if !b.changed || r.Start.Line < b.changedFrom {
		b.changedFrom = r.Start.Line
	}
	b.changed = true
	b.cursor = b.clampPos(b.cursor)
	b.anchor = b.clampPos(b.anchor)
}
