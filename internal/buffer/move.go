package buffer

import (
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Cursor is the caret position.
func (b *Buffer) Cursor() Pos { return b.cursor }

// Selection is the selected range; ok only when an anchor is set and differs from
// the cursor.
func (b *Buffer) Selection() (Range, bool) {
	if !b.hasAnchor || b.anchor == b.cursor {
		return Range{}, false
	}
	if b.cursor.Less(b.anchor) {
		return Range{b.cursor, b.anchor}, true
	}
	return Range{b.anchor, b.cursor}, true
}

// MoveTo moves the cursor to p (clamped). With extend it sets the anchor if none is
// set and keeps it; without, it clears the selection. It closes the coalescing group.
func (b *Buffer) MoveTo(p Pos, extend bool) {
	b.kind = coalesceNone
	if extend {
		if !b.hasAnchor {
			b.anchor, b.hasAnchor = b.cursor, true
		}
	} else {
		b.hasAnchor = false
	}
	b.cursor = b.clampPos(p)
}

// SelectAll selects the whole buffer with the cursor at its end.
func (b *Buffer) SelectAll() {
	b.kind = coalesceNone
	b.anchor, b.hasAnchor = Pos{}, true
	b.cursor = b.End()
}

// step is the byte length of the grapheme starting at s[0]; an invalid UTF-8 byte is
// its own step.
func step(s string) int {
	if r, n := utf8.DecodeRuneInString(s); r == utf8.RuneError && n <= 1 {
		return 1
	}
	g, _, _, _ := uniseg.FirstGraphemeClusterInString(s, -1)
	return len(g)
}

// Left moves one grapheme back, or to the end of the previous line.
func (b *Buffer) Left(p Pos) Pos {
	p = b.clampPos(p)
	if p.Col == 0 {
		if p.Line == 0 {
			return p
		}
		return Pos{p.Line - 1, len(b.lines[p.Line-1])}
	}
	line := b.lines[p.Line]
	prev := 0
	for c := 0; c < p.Col; {
		prev = c
		c += step(line[c:])
	}
	return Pos{p.Line, prev}
}

// Right moves one grapheme forward, or to the start of the next line.
func (b *Buffer) Right(p Pos) Pos {
	p = b.clampPos(p)
	line := b.lines[p.Line]
	if p.Col >= len(line) {
		if p.Line == len(b.lines)-1 {
			return p
		}
		return Pos{p.Line + 1, 0}
	}
	return Pos{p.Line, p.Col + step(line[p.Col:])}
}

// WordLeft skips whitespace then non-whitespace backwards; at column 0 it goes to the
// end of the previous line.
func (b *Buffer) WordLeft(p Pos) Pos {
	p = b.clampPos(p)
	if p.Col == 0 {
		if p.Line == 0 {
			return p
		}
		return Pos{p.Line - 1, len(b.lines[p.Line-1])}
	}
	line := b.lines[p.Line]
	c := p.Col
	for c > 0 {
		r, n := utf8.DecodeLastRuneInString(line[:c])
		if !unicode.IsSpace(r) {
			break
		}
		c -= n
	}
	for c > 0 {
		r, n := utf8.DecodeLastRuneInString(line[:c])
		if unicode.IsSpace(r) {
			break
		}
		c -= n
	}
	return Pos{p.Line, c}
}

// WordRight skips whitespace then non-whitespace forwards; at line end it goes to the
// start of the next line.
func (b *Buffer) WordRight(p Pos) Pos {
	p = b.clampPos(p)
	line := b.lines[p.Line]
	if p.Col >= len(line) {
		if p.Line == len(b.lines)-1 {
			return p
		}
		return Pos{p.Line + 1, 0}
	}
	c := p.Col
	for c < len(line) {
		r, n := utf8.DecodeRuneInString(line[c:])
		if !unicode.IsSpace(r) {
			break
		}
		c += n
	}
	for c < len(line) {
		r, n := utf8.DecodeRuneInString(line[c:])
		if unicode.IsSpace(r) {
			break
		}
		c += n
	}
	return Pos{p.Line, c}
}

// LineStart is the start of p's line.
func (b *Buffer) LineStart(p Pos) Pos {
	return Pos{b.clampPos(p).Line, 0}
}

// LineEnd is the end of p's line.
func (b *Buffer) LineEnd(p Pos) Pos {
	p = b.clampPos(p)
	return Pos{p.Line, len(b.lines[p.Line])}
}
