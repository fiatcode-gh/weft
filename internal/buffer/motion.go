package buffer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The motions of the editor's modal mode: pure functions of the text. They take
// and return byte-column positions on grapheme boundaries, clamp their input,
// never move the cursor and never mutate the buffer.

// LastLine is the last line a modal cursor may rest on: the empty line after a
// final line break is not counted unless it is the only line.
func (b *Buffer) LastLine() int { return b.docLines() - 1 }

// LineLast is the start of the last grapheme of p's line; column 0 when empty.
func (b *Buffer) LineLast(p Pos) Pos {
	p = b.clampPos(p)
	line := b.lines[p.Line]
	last := 0
	for c := 0; c < len(line); {
		last = c
		c += step(line[c:])
	}
	return Pos{p.Line, last}
}

// FirstNonBlank is the first grapheme of p's line that is not a space or tab;
// LineLast on a line of only blanks.
func (b *Buffer) FirstNonBlank(p Pos) Pos {
	p = b.clampPos(p)
	line := b.lines[p.Line]
	for c := 0; c < len(line); c += step(line[c:]) {
		if line[c] != ' ' && line[c] != '\t' {
			return Pos{p.Line, c}
		}
	}
	return b.LineLast(p)
}

// StepRight moves up to n graphemes right without leaving the line (it may
// reach the line end); got is how many it moved.
func (b *Buffer) StepRight(p Pos, n int) (q Pos, got int) {
	q = b.clampPos(p)
	line := b.lines[q.Line]
	for ; got < max(n, 1) && q.Col < len(line); got++ {
		q.Col += step(line[q.Col:])
	}
	return q, got
}

// StepLeft moves up to n graphemes left without leaving the line.
func (b *Buffer) StepLeft(p Pos, n int) (q Pos, got int) {
	q = b.clampPos(p)
	line := b.lines[q.Line]
	for ; got < max(n, 1) && q.Col > 0; got++ {
		q.Col = prevStart(line, q.Col)
	}
	return q, got
}

const (
	clsBlank = iota
	clsWord
	clsPunct
)

// classAt is the class of the grapheme at line[col] by its first rune; the line
// end (a line break) is blank. big merges word and punctuation.
func classAt(line string, col int, big bool) int {
	if col >= len(line) {
		return clsBlank
	}
	r, n := utf8.DecodeRuneInString(line[col:])
	switch {
	case r == utf8.RuneError && n <= 1:
		// An invalid byte is punctuation.
	case unicode.IsSpace(r):
		return clsBlank
	case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_':
		return clsWord
	}
	if big {
		return clsWord
	}
	return clsPunct
}

// prevStart is the start of the grapheme that ends at line[col], col > 0.
func prevStart(line string, col int) int {
	prev := 0
	for c := 0; c < col; {
		prev = c
		c += step(line[c:])
	}
	return prev
}

// lineEndPos is the end of line l.
func (b *Buffer) lineEndPos(l int) Pos { return Pos{l, len(b.lines[l])} }

// modalClamp clamps p into the buffer and above the final empty line.
func (b *Buffer) modalClamp(p Pos) Pos {
	p = b.clampPos(p)
	if last := b.LastLine(); p.Line > last {
		p = Pos{last, min(p.Col, len(b.lines[last]))}
	}
	return p
}

// WordStart is vim's w (W when big), n times.
func (b *Buffer) WordStart(p Pos, n int, big bool) Pos {
	p = b.modalClamp(p)
	for range max(n, 1) {
		q, end := b.wordStart1(p, big)
		p = q
		if end {
			break
		}
	}
	return p
}

// wordStart1 is one w; end reports that nothing follows.
func (b *Buffer) wordStart1(p Pos, big bool) (q Pos, end bool) {
	last := b.LastLine()
	line := b.lines[p.Line]
	if c := classAt(line, p.Col, big); c != clsBlank {
		for p.Col < len(line) && classAt(line, p.Col, big) == c {
			p.Col += step(line[p.Col:])
		}
	}
	for {
		line = b.lines[p.Line]
		if p.Col < len(line) {
			if classAt(line, p.Col, big) != clsBlank {
				return p, false
			}
			p.Col += step(line[p.Col:])
			continue
		}
		if p.Line >= last {
			return b.lineEndPos(last), true
		}
		p = Pos{p.Line + 1, 0}
		if b.lines[p.Line] == "" {
			return p, false
		}
	}
}

// WordEnd is vim's e (E when big), n times.
func (b *Buffer) WordEnd(p Pos, n int, big bool) Pos {
	p = b.modalClamp(p)
	for range max(n, 1) {
		q, end := b.wordEnd1(p, big)
		p = q
		if end {
			break
		}
	}
	return p
}

// wordEnd1 is one e; end reports that nothing follows.
func (b *Buffer) wordEnd1(p Pos, big bool) (q Pos, end bool) {
	last := b.LastLine()
	line := b.lines[p.Line]
	switch {
	case p.Col < len(line):
		p.Col += step(line[p.Col:])
	case p.Line < last:
		p = Pos{p.Line + 1, 0}
	default:
		return b.LineLast(p), true
	}
	for {
		line = b.lines[p.Line]
		if p.Col < len(line) {
			if classAt(line, p.Col, big) != clsBlank {
				break
			}
			p.Col += step(line[p.Col:])
			continue
		}
		if p.Line >= last {
			return b.LineLast(Pos{last, 0}), true
		}
		p = Pos{p.Line + 1, 0}
	}
	c := classAt(line, p.Col, big)
	for {
		nx := p.Col + step(line[p.Col:])
		if nx >= len(line) || classAt(line, nx, big) != c {
			return p, false
		}
		p.Col = nx
	}
}

// WordBack is vim's b (B when big), n times.
func (b *Buffer) WordBack(p Pos, n int, big bool) Pos {
	p = b.modalClamp(p)
	for range max(n, 1) {
		q := b.wordBack1(p, big)
		if q == p {
			break
		}
		p = q
	}
	return p
}

// wordBack1 is one b.
func (b *Buffer) wordBack1(p Pos, big bool) Pos {
	if p == (Pos{}) {
		return p
	}
	q := p
	if q.Col > 0 {
		q.Col = prevStart(b.lines[q.Line], q.Col)
	} else {
		q = b.lineEndPos(q.Line - 1)
	}
	for {
		line := b.lines[q.Line]
		if q.Col >= len(line) {
			if len(line) == 0 {
				return q
			}
			q.Col = prevStart(line, len(line))
			continue
		}
		if classAt(line, q.Col, big) != clsBlank {
			break
		}
		switch {
		case q.Col > 0:
			q.Col = prevStart(line, q.Col)
		case q.Line == 0:
			return q
		default:
			q = b.lineEndPos(q.Line - 1)
		}
	}
	line := b.lines[q.Line]
	c := classAt(line, q.Col, big)
	start := 0
	for i := 0; i < q.Col; {
		s := step(line[i:])
		if classAt(line, i, big) != c {
			start = i + s
		}
		i += s
	}
	return Pos{q.Line, start}
}

// FindInLine finds the nth grapheme equal to g on p's line: kind 'f' lands on
// it, 't' one grapheme before it, 'F'/'T' the same backwards. With repeat a
// 't'/'T' target adjacent to p is skipped (vim ';' after t).
func (b *Buffer) FindInLine(p Pos, n int, g string, kind byte, repeat bool) (Pos, bool) {
	p = b.clampPos(p)
	if g == "" {
		return p, false
	}
	n = max(n, 1)
	line := b.lines[p.Line]
	switch kind {
	case 'f', 't':
		if p.Col >= len(line) {
			return p, false
		}
		prev := p.Col
		c := p.Col + step(line[p.Col:])
		for c < len(line) {
			s := step(line[c:])
			if line[c:c+s] == g && !(kind == 't' && repeat && prev == p.Col) {
				if n--; n == 0 {
					if kind == 'f' {
						return Pos{p.Line, c}, true
					}
					return Pos{p.Line, prev}, true
				}
			}
			prev = c
			c += s
		}
	case 'F', 'T':
		eligible := func(c, s int) bool {
			return line[c:c+s] == g && !(kind == 'T' && repeat && c+s == p.Col)
		}
		total := 0
		for c := 0; c < p.Col; {
			s := step(line[c:])
			if eligible(c, s) {
				total++
			}
			c += s
		}
		skip := total - n
		if skip < 0 {
			return p, false
		}
		for c := 0; c < p.Col; {
			s := step(line[c:])
			if eligible(c, s) {
				if skip == 0 {
					if kind == 'F' {
						return Pos{p.Line, c}, true
					}
					return Pos{p.Line, c + s}, true
				}
				skip--
			}
			c += s
		}
	}
	return p, false
}

// MatchBracket finds the first of ()[]{} at or after p on p's line and returns
// its partner, across lines, nested pairs of the same kind skipped.
func (b *Buffer) MatchBracket(p Pos) (Pos, bool) {
	p = b.clampPos(p)
	const brackets = "()[]{}"
	line := b.lines[p.Line]
	for i := p.Col; i < len(line); i++ {
		k := strings.IndexByte(brackets, line[i])
		if k < 0 {
			continue
		}
		self, other := line[i], brackets[k^1]
		depth := 1
		if k%2 == 0 {
			for l, c := p.Line, i+1; l < len(b.lines); l, c = l+1, 0 {
				text := b.lines[l]
				for ; c < len(text); c++ {
					switch text[c] {
					case self:
						depth++
					case other:
						if depth--; depth == 0 {
							return Pos{l, c}, true
						}
					}
				}
			}
			return p, false
		}
		for l, c := p.Line, i-1; l >= 0; l-- {
			text := b.lines[l]
			if l != p.Line {
				c = len(text) - 1
			}
			for ; c >= 0; c-- {
				switch text[c] {
				case self:
					depth++
				case other:
					if depth--; depth == 0 {
						return Pos{l, c}, true
					}
				}
			}
		}
		return p, false
	}
	return p, false
}

// ParagraphForward is vim's }, n times: the next blank line after the
// paragraph, or past the last character when there is none.
func (b *Buffer) ParagraphForward(p Pos, n int) Pos {
	p = b.modalClamp(p)
	last := b.LastLine()
	for range max(n, 1) {
		l := p.Line
		for l <= last && isBlank(b.lines[l]) {
			l++
		}
		for l <= last && !isBlank(b.lines[l]) {
			l++
		}
		if l > last {
			return b.lineEndPos(last)
		}
		p = Pos{l, 0}
	}
	return p
}

// ParagraphBack is vim's {, n times: the previous blank line before the
// paragraph, or the start of the buffer when there is none.
func (b *Buffer) ParagraphBack(p Pos, n int) Pos {
	p = b.modalClamp(p)
	for range max(n, 1) {
		l := p.Line
		for l >= 0 && isBlank(b.lines[l]) {
			l--
		}
		for l >= 0 && !isBlank(b.lines[l]) {
			l--
		}
		if l < 0 {
			return Pos{}
		}
		p = Pos{l, 0}
	}
	return p
}
