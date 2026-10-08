package render

import (
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// LineRows is what the live preview draws for one source line when it is not
// revealed: Lead (margin rows above its content), Body (its content rows) and
// Trail (margin rows after the document's last content). Every row of the
// document belongs to exactly one line, so the editor draws a line as Lead,
// then its raw rows if revealed or Body otherwise, then Trail.
type LineRows struct{ Lead, Body, Trail []string }

// chunkAttr says which line each content row of a chunk belongs to. Lines are
// relative to the chunk's first line. A row is content when it shows
// something, margin when it is blank.
type chunkAttr struct {
	line     []int          // per row: its line when it is a content row, else -1
	content  []int          // indexes of the content rows
	span     map[int][2]int // line: its first and last content row
	fallback bool           // the row map is unusable: everything is on one line
}

// attribution returns the chunk's row attribution (rules 1-3 and 7 of the
// plan), built once; nil for a chunk that failed to render.
func (cr *chunkRender) attribution() *chunkAttr {
	if cr.err != nil {
		return nil
	}
	if cr.attr == nil {
		cr.attr = cr.attribute()
	}
	return cr.attr
}

func (cr *chunkRender) attribute() *chunkAttr {
	n := len(cr.rows)
	a := &chunkAttr{line: make([]int, n), span: map[int][2]int{}}
	isContent := make([]bool, n)
	for r, row := range cr.rows {
		isContent[r] = strings.TrimSpace(ansi.Strip(row)) != ""
		a.line[r] = -1
	}
	hasMap := len(cr.lines) == n && n > 0
	a.fallback = cr.diverged || (!hasMap && n > 0)

	// A content row that is not its line's own (a table border, a wrapped
	// letterless fragment) joins the row above it, else the next own row.
	nextOwn := make([]int, n)
	next := 0
	for r := n - 1; r >= 0 && hasMap; r-- {
		nextOwn[r] = next
		if isContent[r] && cr.own[r] {
			next = cr.lines[r]
		}
	}
	for r := range n {
		if !isContent[r] {
			continue
		}
		switch {
		case a.fallback:
			a.line[r] = 0
		case cr.own[r]:
			a.line[r] = cr.lines[r]
		case r > 0 && isContent[r-1]:
			a.line[r] = a.line[r-1]
		default:
			a.line[r] = nextOwn[r]
		}
		if k := len(a.content); k > 0 && a.line[r] < a.line[a.content[k-1]] {
			// Not monotone: no row map to trust.
			a.fallback = true
			for _, q := range a.content {
				a.line[q] = 0
			}
			a.line[r] = 0
		}
		a.content = append(a.content, r)
	}
	for _, r := range a.content {
		l := a.line[r]
		if sp, ok := a.span[l]; ok {
			a.span[l] = [2]int{sp[0], r}
		} else {
			a.span[l] = [2]int{r, r}
		}
	}
	return a
}

// rendered is render, remembered per buffer version: the editor asks for every
// line of the window in turn.
func (p *Preview) rendered(src PreviewLines, sc *Scanner, c chunkSpan) *chunkRender {
	if p.memoAt != p.version || p.memo == nil {
		p.memo, p.memoAt = map[int]memoEntry{}, p.version
	}
	if m, ok := p.memo[c.from]; ok && m.to == c.to {
		return m.cr
	}
	cr := p.render(src, sc, c)
	if cr.err == nil {
		p.memo[c.from] = memoEntry{to: c.to, cr: cr}
	}
	return cr
}

// memoEntry is a rendered chunk remembered by rendered.
type memoEntry struct {
	to int
	cr *chunkRender
}

// rowPos is a content row of a chunk, or the start of the document or of the
// stretch after a chunk that failed to render (row == -1).
type rowPos struct {
	c     chunkSpan
	cr    *chunkRender
	row   int
	line  int  // the row's line; else the line just before the stretch
	start bool // row == -1 at the start of the document
}

// prevContent finds the last content row before row `before` of chunk c, in
// c or the chunks before it.
func (p *Preview) prevContent(src PreviewLines, sc *Scanner, c chunkSpan, cr *chunkRender, before int) rowPos {
	first, _ := p.bounds(src, sc)
	for {
		a := cr.attr
		for r := before - 1; r >= 0; r-- {
			if a.line[r] >= 0 {
				return rowPos{c: c, cr: cr, row: r, line: c.from + a.line[r]}
			}
		}
		if c.from <= first {
			return rowPos{c: c, cr: cr, row: -1, line: first - 1, start: true}
		}
		pc, _ := p.chunkAt(src, sc, c.from-1)
		pcr := p.rendered(src, sc, pc)
		if pcr.err != nil {
			return rowPos{c: c, cr: cr, row: -1, line: pc.to - 1}
		}
		pcr.attribution()
		c, cr, before = pc, pcr, len(pcr.rows)
	}
}

// runFrom collects the margin rows after pos up to the next content row, which
// may be in a later chunk. b is that row's line, -1 when the document (or the
// stretch before a chunk that failed) ends first, and hi is the line bounding
// the run: b, or the first line past the stretch.
func (p *Preview) runFrom(src PreviewLines, sc *Scanner, pos rowPos) (run []string, b, hi int) {
	c, cr := pos.c, pos.cr
	if pos.start && p.head > 1 {
		run = append(run, p.headRows[1:]...)
	}
	for r := pos.row + 1; ; {
		a := cr.attr
		for ; r < len(cr.rows); r++ {
			if a.line[r] >= 0 {
				return run, c.from + a.line[r], c.from + a.line[r]
			}
			run = append(run, cr.rows[r])
		}
		_, last := p.bounds(src, sc)
		if c.to > last {
			return run, -1, last + 1
		}
		nc, _ := p.chunkAt(src, sc, c.to)
		ncr := p.rendered(src, sc, nc)
		if ncr.err != nil {
			return run, -1, nc.from
		}
		ncr.attribution()
		c, cr, r = nc, ncr, 0
	}
}

// pairable lists the lines strictly between lo and hi that a margin row can
// be paired with: blank lines (hidden blocks drop theirs from the render).
func pairable(src PreviewLines, sc *Scanner, lo, hi int) []int {
	var z []int
	for k := max(lo+1, 0); k < hi && k < src.Len(); k++ {
		if strings.Trim(src.Line(k), " \t") == "" && sc.Info(src, k).Kind != KindHidden {
			z = append(z, k)
		}
	}
	return z
}

// Rows returns the rows the live preview draws for line i when it is not
// revealed. false means the line's chunk failed to render: draw it raw.
//
// Every content row is the Body of the line the row map gives it. A run of
// margin rows between two content lines is paired, one row each, with the
// blank lines between them (each becomes that line's Body); the rest are the
// Lead of the line below. A run that ends the document (or the stretch before
// a chunk that failed) pairs the same way and its rest is the Trail of the
// stretch's last line, so the rows stay in display order. A chunk whose map is
// unusable keeps all its content in its first line's Body.
func (p *Preview) Rows(src PreviewLines, sc *Scanner, i int) (LineRows, bool) {
	c, ok := p.chunkAt(src, sc, i)
	if !ok {
		return LineRows{}, true
	}
	cr := p.rendered(src, sc, c)
	a := cr.attribution()
	if a == nil {
		return LineRows{}, false
	}
	var lr LineRows
	rel := i - c.from
	if sp, has := a.span[rel]; has {
		lr.Body = cr.rows[sp[0] : sp[1]+1]
		pos := p.prevContent(src, sc, c, cr, sp[0])
		run, _, hi := p.runFrom(src, sc, pos)
		lr.Lead = unpaired(run, pairable(src, sc, pos.line, hi))
	} else if strings.Trim(src.Line(i), " \t") == "" && sc.Info(src, i).Kind != KindHidden {
		// A blank line owns the margin row paired with it.
		k := sort.Search(len(a.content), func(k int) bool { return a.line[a.content[k]] > rel })
		before := len(cr.rows)
		if k < len(a.content) {
			before = a.content[k]
		}
		pos := p.prevContent(src, sc, c, cr, before)
		run, _, hi := p.runFrom(src, sc, pos)
		z := pairable(src, sc, pos.line, hi)
		if j := sort.SearchInts(z, i); j < len(run) && j < len(z) && z[j] == i {
			lr.Body = run[j : j+1]
		}
	}
	if i == c.to-1 {
		pos := p.prevContent(src, sc, c, cr, len(cr.rows))
		run, b, hi := p.runFrom(src, sc, pos)
		if b < 0 && hi-1 == i {
			lr.Trail = unpaired(run, pairable(src, sc, pos.line, hi))
		}
	}
	return lr, true
}

// unpaired is the rows of run left once the first len(z) are paired with the
// lines of z.
func unpaired(run []string, z []int) []string {
	return run[min(len(z), len(run)):]
}

// Unit returns the lines [from, to) that are revealed together with line i:
// the lines of a table, a fence, a quote with its lazy continuation lines or a
// hidden block, a setext heading with its underline, and lines that share a
// rendered row, merged transitively. A line outside the trimmed document or in
// a chunk that failed to render is its own unit; a chunk whose row map is
// unusable is one unit.
func (p *Preview) Unit(src PreviewLines, sc *Scanner, i int) (from, to int) {
	c, ok := p.chunkAt(src, sc, i)
	if !ok {
		return i, i + 1
	}
	cr := p.rendered(src, sc, c)
	a := cr.attribution()
	if a == nil {
		return i, i + 1
	}
	if a.fallback {
		return c.from, c.to
	}

	var iv [][2]int
	runStart, runKind := c.from, KindBlank
	endRun := func(k int) {
		if k-1 > runStart && (runKind == KindTable || runKind == KindQuote || runKind == KindHidden) {
			iv = append(iv, [2]int{runStart, k - 1})
		}
	}
	lazy := false // the quote's last line ends in paragraph text, so a plain text line below continues it
	for k := c.from; k < c.to; k++ {
		info := sc.Info(src, k)
		line := src.Line(k)
		continues := runKind == KindQuote && lazy && info.Kind == KindText && !info.InList && paraLike(line)
		if !continues && (info.Kind != runKind || (info.Kind != KindTable && info.Kind != KindQuote && info.Kind != KindHidden)) {
			endRun(k)
			runStart, runKind = k, info.Kind
		}
		if info.Kind == KindQuote {
			lazy = quoteEndsInText(line)
		} else if !continues {
			lazy = false
		}
		switch info.Kind {
		case KindFence, KindCode:
			iv = append(iv, [2]int{max(info.Fence, c.from), k})
		case KindSetextUnderline:
			iv = append(iv, [2]int{max(k-1, c.from), k})
		}
	}
	endRun(c.to)
	for _, s := range cr.share {
		iv = append(iv, [2]int{c.from + s[0], c.from + s[1]})
	}

	sort.Slice(iv, func(x, y int) bool { return iv[x][0] < iv[y][0] })
	for k := 0; k < len(iv); k++ {
		cur := iv[k]
		for k+1 < len(iv) && iv[k+1][0] <= cur[1] {
			k++
			cur[1] = max(cur[1], iv[k][1])
		}
		if cur[0] <= i && i <= cur[1] {
			return max(cur[0], c.from), min(cur[1]+1, c.to)
		}
	}
	return i, i + 1
}

// quoteEndsInText reports whether the quote line ends in plain paragraph text,
// which a following line without '>' continues (a lazy continuation line): its
// content after the marker is not blank, not indented code and not the start
// of another block.
func quoteEndsInText(line string) bool {
	content := line[len(quoteRe.FindString(line)):]
	return strings.TrimSpace(content) != "" && !strings.HasPrefix(content, "    ") && !strings.HasPrefix(content, "\t") && paraLike(content)
}
