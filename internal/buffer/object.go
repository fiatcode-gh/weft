package buffer

// The text objects of the editor's modal mode: pure functions of the text,
// byte positions on grapheme boundaries, ranges end-exclusive, no mutation.

// span is a run [start, end) of one kind: a word-object run in a line or a
// paragraph run in lines.
type span struct {
	start, end int
	cls        int // class of a word run; unused for paragraphs
	blank      bool
}

// aroundSpans is the unit run for "a" objects: n units starting at run i; a
// unit is its run plus the next run when that is of the other kind (blank vs
// not). trailed reports that the last unit took such a next run.
func aroundSpans(runs []span, i, n int) (lo, hi int, trailed bool) {
	lo, hi = i, i
	for j := i; n > 0 && j < len(runs); n-- {
		hi, trailed = j, false
		if j+1 < len(runs) && runs[j].blank != runs[j+1].blank {
			hi, trailed = j+1, true
		}
		j = hi + 1
	}
	return lo, hi, trailed
}

// innerSpans is n runs starting at run i, clipped at the last run.
func innerSpans(runs []span, i, n int) (lo, hi int) {
	return i, min(i+n-1, len(runs)-1)
}

// WordObject is iw/aw (big: iW/aW) at p, line-local.
func (b *Buffer) WordObject(p Pos, n int, around, big bool) (Range, bool) {
	p = b.clampPos(p)
	line := b.lines[p.Line]
	if line == "" {
		return Range{}, false
	}
	var runs []span
	at := -1
	for c := 0; c < len(line); {
		cls := classAt(line, c, big)
		if len(runs) == 0 || runs[len(runs)-1].cls != cls {
			runs = append(runs, span{start: c, cls: cls, blank: cls == clsBlank})
		}
		c += step(line[c:])
		runs[len(runs)-1].end = c
		if p.Col >= runs[len(runs)-1].start && (p.Col < c || c == len(line)) {
			at = len(runs) - 1
		}
	}
	n = max(n, 1)
	var lo, hi int
	if !around {
		lo, hi = innerSpans(runs, at, n)
	} else {
		var trailed bool
		lo, hi, trailed = aroundSpans(runs, at, n)
		if !trailed && !runs[at].blank && at > 0 && runs[at-1].blank {
			lo = at - 1
		}
	}
	return Range{Pos{p.Line, runs[lo].start}, Pos{p.Line, runs[hi].end}}, true
}

// QuoteObject is i"/a" (quote '"' or '\”) on p's line.
func (b *Buffer) QuoteObject(p Pos, quote byte, around bool) (Range, bool) {
	p = b.clampPos(p)
	line := b.lines[p.Line]
	var qs []int
	for c, slashes := 0, 0; c < len(line); c++ {
		switch {
		case line[c] == '\\':
			slashes++
			continue
		case line[c] == quote && slashes%2 == 0:
			qs = append(qs, c)
		}
		slashes = 0
	}
	open, closeAt := -1, -1
	for i := 0; i+1 < len(qs); i += 2 {
		// The first pair ending at or after p contains p or is the next one.
		if qs[i+1] >= p.Col {
			open, closeAt = qs[i], qs[i+1]
			break
		}
	}
	if open < 0 {
		return Range{}, false
	}
	if !around {
		return Range{Pos{p.Line, open + 1}, Pos{p.Line, closeAt}}, true
	}
	start, end := open, closeAt+1
	if end < len(line) && (line[end] == ' ' || line[end] == '\t') {
		for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
			end++
		}
	} else {
		for start > 0 && (line[start-1] == ' ' || line[start-1] == '\t') {
			start--
		}
	}
	return Range{Pos{p.Line, start}, Pos{p.Line, end}}, true
}

func closerOf(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	}
	return '}'
}

// openerBefore is the nearest unmatched open before q, balanced pairs skipped.
func (b *Buffer) openerBefore(q Pos, open, closer byte) (Pos, bool) {
	depth := 0
	for l, c := q.Line, q.Col-1; l >= 0; {
		if c < 0 {
			if l--; l >= 0 {
				c = len(b.lines[l]) - 1
			}
			continue
		}
		switch b.lines[l][c] {
		case closer:
			depth++
		case open:
			if depth == 0 {
				return Pos{l, c}, true
			}
			depth--
		}
		c--
	}
	return Pos{}, false
}

// closerAfter is the partner of the open at o.
func (b *Buffer) closerAfter(o Pos, open, closer byte) (Pos, bool) {
	depth := 0
	for l, c := o.Line, o.Col+1; l < len(b.lines); l, c = l+1, 0 {
		for ; c < len(b.lines[l]); c++ {
			switch b.lines[l][c] {
			case open:
				depth++
			case closer:
				if depth == 0 {
					return Pos{l, c}, true
				}
				depth--
			}
		}
	}
	return Pos{}, false
}

// BracketObject is i(/a( for open '(' '[' '{': the nth pair enclosing p.
func (b *Buffer) BracketObject(p Pos, n int, open byte, around bool) (Range, bool) {
	p = b.clampPos(p)
	closer := closerOf(open)
	var o, c Pos
	var ok bool
	if line := b.lines[p.Line]; p.Col < len(line) && line[p.Col] == open {
		o = p
	} else if o, ok = b.openerBefore(p, open, closer); !ok {
		return Range{}, false
	}
	for k := max(n, 1); ; k-- {
		if c, ok = b.closerAfter(o, open, closer); !ok {
			return Range{}, false
		}
		if k == 1 {
			break
		}
		if o, ok = b.openerBefore(o, open, closer); !ok {
			return Range{}, false
		}
	}
	oEnd := Pos{o.Line, o.Col + step(b.lines[o.Line][o.Col:])}
	if around {
		return Range{o, Pos{c.Line, c.Col + step(b.lines[c.Line][c.Col:])}}, true
	}
	start, end := oEnd, c
	if o.Line < c.Line {
		if oEnd.Col == len(b.lines[o.Line]) {
			start = Pos{o.Line + 1, 0}
		}
		if isBlank(b.lines[c.Line][:c.Col]) {
			end = b.lineEndPos(c.Line - 1)
		}
	}
	if end.Less(start) {
		end = oEnd
		start = oEnd
	}
	return Range{start, end}, true
}

// ParagraphObject is ip/ap: a line span, linewise.
func (b *Buffer) ParagraphObject(p Pos, n int, around bool) (first, last int, ok bool) {
	p = b.modalClamp(p)
	var runs []span
	at := 0
	for l := 0; l <= b.LastLine(); l++ {
		blank := isBlank(b.lines[l])
		if len(runs) > 0 && runs[len(runs)-1].blank == blank {
			runs[len(runs)-1].end = l + 1
		} else {
			runs = append(runs, span{start: l, end: l + 1, blank: blank})
		}
		if l == p.Line {
			at = len(runs) - 1
		}
	}
	n = max(n, 1)
	var lo, hi int
	if !around {
		lo, hi = innerSpans(runs, at, n)
	} else {
		var trailed bool
		lo, hi, trailed = aroundSpans(runs, at, n)
		if !trailed && !runs[at].blank && at > 0 {
			lo = at - 1
		}
	}
	return runs[lo].start, runs[hi].end - 1, true
}
