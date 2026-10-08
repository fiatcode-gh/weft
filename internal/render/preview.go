package render

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	gtext "github.com/yuin/goldmark/text"
)

// PreviewLines is a document with its line terminators: the chunk text is each
// line plus its real terminator, because the read view renders a CRLF page
// differently from an LF one.
type PreviewLines interface {
	SourceLines
	Terminator(i int) string
}

// PreviewStats counts the real Glamour renders of chunks (cache misses) and
// the source lines in them.
type PreviewStats struct{ Chunks, Lines int }

// chunkSpan is the source lines [from, to) of a chunk.
type chunkSpan struct{ from, to int }

// chunkRender is a rendered chunk: its own rows, or the reason it has none.
// lines and own map each row to a line relative to the chunk's first line (see
// sourceRows); share holds the relative line intervals that share a rendered
// row; diverged marks a chunk whose row map is missing or unreliable.
type chunkRender struct {
	rows     []string
	err      error
	lines    []int
	own      []bool
	share    [][2]int
	diverged bool
	attr     *chunkAttr // built on first use by attribution
}

// chunkFacts is what the sandwich needs to know about a chunk's own markdown.
type chunkFacts struct {
	nodes    int      // top-level blocks, of any kind
	lastList byte     // marker of the last block when it is an unordered list, else 0
	refs     []string // link reference definitions, serialised
}

const (
	// Private-use stubs the sandwich puts above (open) and below (close) the
	// chunk; the chunk's rows are the ones between the rows they land on.
	stubOpen  = "\ue020"
	stubClose = "\ue022"

	previewCacheLimit = 256
)

var refCandidateRe = regexp.MustCompile(`^[ \t>*+\-0-9.)]*\[[^\[\]]`)

// genCache is the two-generation map of fenceCache: a hit is promoted, and a
// full current generation replaces the previous one.
type genCache[V any] struct{ cur, prev map[string]V }

func (c *genCache[V]) get(k string) (V, bool) {
	if v, ok := c.cur[k]; ok {
		return v, true
	}
	if v, ok := c.prev[k]; ok {
		c.put(k, v)
		return v, true
	}
	var zero V
	return zero, false
}

func (c *genCache[V]) put(k string, v V) {
	if c.cur == nil || len(c.cur) >= previewCacheLimit {
		c.prev, c.cur = c.cur, make(map[string]V, previewCacheLimit)
	}
	c.cur[k] = v
}

// refEntry is the definitions one chunk carries.
type refEntry struct {
	from int
	refs []string
}

// Preview renders the chunks of a document for the editor's live preview.
//
// A chunk is a run of source lines that starts at a chunk start (chunkState)
// and ends before the next one. It is rendered on its own by the read view's
// own pipeline, wrapped in a sandwich that recreates what the whole document
// gives it, and the chunk's own rows are cut out of the result, so every row is
// byte-identical to the row Render gives the same line in the whole page.
// Rendering the whole document per keystroke would cost 0.4-0.7 s at 10000
// lines; a chunk costs about 43 µs per source line. See
// docs/decisions/live-preview.md.
//
// A Preview belongs to one width and one document; the owner calls
// Invalidate with the first changed line after every edit, together with
// Scanner.Invalidate.
type Preview struct {
	theme    Theme
	width    int
	head     int // rows above the first row of a document
	headRows []string
	tail     int   // rows below the last row of a document
	err      error // head and tail could not be measured
	stats    PreviewStats

	version int

	starts  []int // chunk starts among lines [0, scanned), ascending
	scanned int

	cum []idBase // cum[k]: sentinels of the lines [0, k)

	lastAt, lastLine int // last non-blank line, valid when lastAt == version
	fcAt, fcLine     int // start of the first chunk with content, valid when fcAt == version
	refsAt           int
	refEntries       []refEntry
	cand             []int // lines that may hold a link reference definition
	candScanned      int

	memo   map[int]memoEntry // rendered chunks by first line, valid when memoAt == version
	memoAt int

	facts   genCache[*chunkFacts]
	renders genCache[*chunkRender]
}

// NewPreview returns a Preview for documents rendered at width. It renders one
// stub to learn how many rows Glamour puts above and below a document.
func NewPreview(t Theme, width int) *Preview {
	p := &Preview{theme: t, width: width, cum: []idBase{{}}, lastAt: -1, fcAt: -1, refsAt: -1}
	p.headRows, p.tail, p.err = p.measure()
	p.head = len(p.headRows)
	return p
}

func (p *Preview) measure() (headRows []string, tail int, err error) {
	pl, err := p.pipeline(stubOpen+"\n", idBase{})
	if err != nil {
		return nil, 0, err
	}
	rows := pl.rows
	for i, row := range rows {
		if strings.Contains(row, stubOpen) {
			return rows[:i], len(rows) - 1 - i, nil
		}
	}
	return nil, 0, errStubMissing
}

// Head is the number of rows above the first row of a document: the read
// view's top margin.
func (p *Preview) Head() int { return p.head }

// Stats returns the work done since creation.
func (p *Preview) Stats() PreviewStats { return p.stats }

// Invalidate drops what depends on lines from from on. Line from-1 goes too,
// as in Scanner.Invalidate.
func (p *Preview) Invalidate(from int) {
	lim := max(0, from-1)
	p.version++
	p.starts = p.starts[:sort.SearchInts(p.starts, lim)]
	p.scanned = min(p.scanned, lim)
	p.cum = p.cum[:min(len(p.cum), lim+1)]
	p.cand = p.cand[:sort.SearchInts(p.cand, lim)]
	p.candScanned = min(p.candScanned, lim)
}

// extend finds the chunk starts among the lines before limit.
func (p *Preview) extend(src SourceLines, sc *Scanner, limit int) {
	for limit = min(limit, src.Len()); p.scanned < limit; p.scanned++ {
		if sc.Info(src, p.scanned).Start {
			p.starts = append(p.starts, p.scanned)
		}
	}
}

// bounds returns the first and last lines of the document the read view
// renders: the first is the first non-blank line, the last is the last one. Both
// are -1 when the document is blank.
func (p *Preview) bounds(src PreviewLines, sc *Scanner) (first, last int) {
	for len(p.starts) == 0 && p.scanned < src.Len() {
		p.extend(src, sc, p.scanned+1)
	}
	if len(p.starts) == 0 {
		return -1, -1
	}
	if p.lastAt != p.version {
		p.lastLine = -1
		for i := src.Len() - 1; i >= 0; i-- {
			if strings.TrimSpace(src.Line(i)) != "" {
				p.lastLine = i
				break
			}
		}
		p.lastAt = p.version
	}
	return p.starts[0], p.lastLine
}

// chunkAt returns the chunk holding line i; false when i is outside the
// trimmed document (blank lines before its first and after its last line).
func (p *Preview) chunkAt(src PreviewLines, sc *Scanner, i int) (chunkSpan, bool) {
	first, last := p.bounds(src, sc)
	if first < 0 || i < first || i > last {
		return chunkSpan{}, false
	}
	p.extend(src, sc, i+1)
	k := sort.SearchInts(p.starts, i+1) - 1
	c := chunkSpan{from: p.starts[k], to: last + 1}
	for {
		if k+1 < len(p.starts) {
			if next := p.starts[k+1]; next <= last {
				c.to = next
			}
			return c, true
		}
		if p.scanned > last {
			return c, true
		}
		p.extend(src, sc, p.scanned+1)
	}
}

// chunkText is the text the read view renders for the chunk: each line with its
// own terminator, with the document's leading and trailing whitespace trimmed.
func (p *Preview) chunkText(src PreviewLines, c chunkSpan, first, last int) string {
	var b strings.Builder
	for i := c.from; i < c.to; i++ {
		line, term := src.Line(i), src.Terminator(i)
		if i == first {
			line = strings.TrimLeftFunc(line, unicode.IsSpace)
		}
		if i == last {
			line, term = strings.TrimRightFunc(line, unicode.IsSpace), "\n"
		}
		b.WriteString(line)
		b.WriteString(term)
	}
	return b.String()
}

// lineSentinels counts the sentinels the read view's substitution passes make
// on one line: wiki links and the task marker.
func lineSentinels(line string) (wiki, task int) {
	if strings.Contains(line, "[[") {
		var subs []linkSubst
		replaceWikiLinksOutsideInlineCode(hideMarkdownLinkURLsOutsideInlineCode(line), 0, &subs)
		wiki = len(subs)
	}
	if taskMarkerRe.MatchString(line) {
		task = 1
	}
	return wiki, task
}

// base is the sentinel ids a chunk starting at from begins with: the number of
// each kind the document has before it.
func (p *Preview) base(src PreviewLines, sc *Scanner, from int) idBase {
	first, _ := p.bounds(src, sc)
	for len(p.cum) <= from {
		n := len(p.cum) - 1
		b := p.cum[n]
		if sc.Info(src, n).Subst {
			line := src.Line(n)
			if n == first {
				line = strings.TrimLeftFunc(line, unicode.IsSpace)
			}
			w, t := lineSentinels(line)
			b.wiki += w
			b.task += t
		}
		p.cum = append(p.cum, b)
	}
	return p.cum[from]
}

// factsOf parses a chunk on its own, as the read view's preprocessing leaves it.
func (p *Preview) factsOf(text string) *chunkFacts {
	if f, ok := p.facts.get(text); ok {
		return f
	}
	src := []byte(preprocess(text, "", idBase{}).pre)
	doc := sourceMarkdown.Parser().Parse(gtext.NewReader(src))
	f := &chunkFacts{nodes: doc.ChildCount()}
	if l, ok := doc.LastChild().(*ast.List); ok && !l.IsOrdered() {
		f.lastList = l.Marker
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if d, ok := n.(*ast.LinkReferenceDefinition); ok && entering {
			if s := serialiseDefinition(d); !strings.ContainsFunc(s, func(r rune) bool { return r >= 0xE000 && r <= 0xE01F }) {
				f.refs = append(f.refs, s)
			}
		}
		return ast.WalkContinue, nil
	})
	p.facts.put(text, f)
	return f
}

func serialiseDefinition(d *ast.LinkReferenceDefinition) string {
	s := "[" + string(d.Label) + "]: <" + string(d.Destination) + ">"
	if d.Title != nil {
		s += ` "` + strings.ReplaceAll(string(d.Title), `"`, `\"`) + `"`
	}
	return s
}

// firstContent is the start of the first chunk, in document order, that parses
// to at least one block; -1 when none does. That chunk and every one before it
// is rendered without a prefix: it is where the document really starts.
func (p *Preview) firstContent(src PreviewLines, sc *Scanner) int {
	if p.fcAt == p.version {
		return p.fcLine
	}
	p.fcLine, p.fcAt = -1, p.version
	first, last := p.bounds(src, sc)
	for i := first; first >= 0 && i <= last; {
		c, _ := p.chunkAt(src, sc, i)
		if p.factsOf(p.chunkText(src, c, first, last)).nodes > 0 {
			p.fcLine = c.from
			break
		}
		i = c.to
	}
	return p.fcLine
}

// definitions lists the link reference definitions of every chunk that holds
// one, in document order.
func (p *Preview) definitions(src PreviewLines, sc *Scanner) []refEntry {
	if p.refsAt == p.version {
		return p.refEntries
	}
	for ; p.candScanned < src.Len(); p.candScanned++ {
		if l := src.Line(p.candScanned); strings.Contains(l, "]:") && refCandidateRe.MatchString(l) {
			p.cand = append(p.cand, p.candScanned)
		}
	}
	p.refEntries, p.refsAt = nil, p.version
	first, last := p.bounds(src, sc)
	done := -1
	for _, line := range p.cand {
		c, ok := p.chunkAt(src, sc, line)
		if !ok || c.from == done {
			continue
		}
		done = c.from
		if refs := p.factsOf(p.chunkText(src, c, first, last)).refs; len(refs) > 0 {
			p.refEntries = append(p.refEntries, refEntry{c.from, refs})
		}
	}
	return p.refEntries
}

// continues reports whether the chunk starting at next carries on the bullet
// list that the chunk before it ends with.
func continues(prev *chunkFacts, nextInfo LineInfo) bool {
	return nextInfo.Bullet != 0 && prev.lastList == nextInfo.Bullet
}

var errStubMissing = errorString("preview: a stub row is missing from the render")

type errorString string

func (e errorString) Error() string { return string(e) }

// pipeline is what one run of the read view's pipeline over a body leaves:
// the finished rows, and the pieces the row map needs.
type pipeline struct {
	rows   []string
	f      frontend
	r      *glamour.TermRenderer
	styled string
}

func (p *Preview) pipeline(body string, base idBase) (pipeline, error) {
	f := preprocess(body, "", base)
	r, err := rendererFor(p.width)
	if err != nil {
		return pipeline{}, err
	}
	styled, err := glamourRender(r, f.pre)
	if err != nil {
		return pipeline{}, err
	}
	return pipeline{rows: strings.Split(finish(styled, f, p.theme, nil).Styled, "\n"), f: f, r: r, styled: styled}, nil
}

// sandwich is what surrounds a chunk's text to recreate its context.
type sandwich struct {
	prefix, suffix string
	refsAfter      string // definitions of later chunks, for the first content chunk
}

// sandwichOf decides the prefix and suffix of chunk c.
//
// The first chunk with content (and every chunk before it) has no prefix: it is
// where the document starts, so Glamour's own first-block rules apply. Any other
// chunk gets a stub block above it so those rules treat it as a later block;
// when it continues the previous chunk's bullet list the stub is an item of that
// list, so the list's rendering carries on. A chunk followed by a continuing
// chunk ends in a stub item, so its last item renders as a non-last one.
// Link reference definitions of the other chunks ride along: before a later
// chunk (the earliest definition wins, as in the whole document), after the
// first one.
func (p *Preview) sandwichOf(src PreviewLines, sc *Scanner, c chunkSpan, first, last int, own *chunkFacts) sandwich {
	var s sandwich
	fc := p.firstContent(src, sc)
	refs := p.definitions(src, sc)
	if fc >= 0 && c.from > fc {
		s.prefix = stubOpen + "\n\n"
		prev, _ := p.chunkAt(src, sc, c.from-1)
		if info := sc.Info(src, c.from); continues(p.factsOf(p.chunkText(src, prev, first, last)), info) {
			s.prefix = string(info.Bullet) + " " + stubOpen + "\n"
		}
		var all []string
		for _, e := range refs {
			all = append(all, e.refs...)
		}
		if len(all) > 0 {
			s.prefix = strings.Join(all, "\n") + "\n\n" + s.prefix
		}
	}
	if c.to <= last {
		if info := sc.Info(src, c.to); continues(own, info) {
			s.suffix = string(info.Bullet) + " " + stubClose + "\n"
		}
	}
	if c.from == fc {
		var later []string
		for _, e := range refs {
			if e.from > fc {
				later = append(later, e.refs...)
			}
		}
		if len(later) > 0 {
			s.refsAfter = "\n" + strings.Join(later, "\n") + "\n"
		}
	}
	return s
}

// cut returns the rows of a render of the sandwiched chunk that belong to the
// chunk: between the rows the stubs land on, or the head and tail rows.
func (p *Preview) cut(rows []string, s sandwich, lastChunk bool) (start, end int, err error) {
	start, end = p.head, len(rows)-p.tail
	if s.prefix != "" {
		if start = rowWith(rows, stubOpen, false) + 1; start == 0 {
			return 0, 0, errStubMissing
		}
	}
	switch {
	case s.suffix != "":
		if end = rowWith(rows, stubClose, true); end < 0 {
			return 0, 0, errStubMissing
		}
	case lastChunk:
		end = len(rows)
	}
	start = min(start, len(rows))
	return start, max(start, end), nil
}

// chunkRowMap is the read view's row map; a variable so tests can disturb it.
var chunkRowMap = sourceRowsDetail

// mapRows fills cr's row map from the pipeline of the sandwiched chunk c, whose
// own rows are pl.rows[start:end].
func (p *Preview) mapRows(cr *chunkRender, src PreviewLines, c chunkSpan, s sandwich, pl pipeline, start, end int, lastChunk bool) {
	nt := c.to - c.from
	only, n := 0, 0
	for k := c.from; k < c.to && n < 2; k++ {
		if strings.TrimSpace(src.Line(k)) != "" {
			only, n = k-c.from, n+1
		}
	}
	if n == 1 {
		cr.lines = make([]int, len(cr.rows))
		cr.own = make([]bool, len(cr.rows))
		for k, row := range cr.rows {
			cr.lines[k], cr.own[k] = only, strings.TrimSpace(ansi.Strip(row)) != ""
		}
		return
	}

	// Each line of pre sits in the sandwiched body: the prefix and the
	// definitions above the chunk belong to its first line, the suffix and the
	// definitions below it to its last.
	np := strings.Count(s.prefix, "\n")
	rel := make([]int, len(pl.f.src))
	for j, b := range pl.f.src {
		switch {
		case b < np:
			rel[j] = 0
		case b < np+nt:
			rel[j] = b - np
		default:
			rel[j] = nt - 1
		}
	}
	m := chunkRowMap(pl.r, pl.styled, pl.f.pre, rel)
	if m.lines == nil || len(m.lines) != len(pl.rows) {
		cr.diverged = true
		return
	}
	cr.lines, cr.own, cr.diverged = m.lines[start:end], m.own[start:end], m.diverged
	if m.diverged {
		return
	}
	ts, te, err := p.cut(m.tagged, s, lastChunk)
	if err != nil || te-ts != end-start {
		cr.diverged = true
		return
	}
	cr.share = rowTagSpans(m.tagged[ts:te], nt-1)
}

// render renders chunk c in its sandwich and returns its own rows. A failure is
// returned, not cached.
func (p *Preview) render(src PreviewLines, sc *Scanner, c chunkSpan) *chunkRender {
	if p.err != nil {
		return &chunkRender{err: p.err}
	}
	first, last := p.bounds(src, sc)
	text := p.chunkText(src, c, first, last)
	base := p.base(src, sc, c.from)
	s := p.sandwichOf(src, sc, c, first, last, p.factsOf(text))
	body := s.prefix + text + s.suffix + s.refsAfter

	// The document's last chunk keeps the rows below its text, so it is cached
	// apart. The row map is relative to the chunk, so its length is part of
	// what it depends on.
	lastChunk := c.to == last+1
	key := strconv.Itoa(base.wiki) + "," + strconv.Itoa(base.task) + "," + strconv.FormatBool(lastChunk) + "," + strconv.Itoa(c.to-c.from) + "\x00" + body
	if r, ok := p.renders.get(key); ok {
		return r
	}
	pl, err := p.pipeline(body, base)
	if err != nil {
		return &chunkRender{err: err}
	}
	start, end, err := p.cut(pl.rows, s, lastChunk)
	if err != nil {
		return &chunkRender{err: err}
	}
	r := &chunkRender{rows: pl.rows[start:end]}
	p.mapRows(r, src, c, s, pl, start, end, lastChunk)
	p.stats.Chunks++
	p.stats.Lines += c.to - c.from
	p.renders.put(key, r)
	return r
}

// rowWith is the index of the first (or, with fromEnd, the last) row holding
// stub, or -1.
func rowWith(rows []string, stub string, fromEnd bool) int {
	if fromEnd {
		for i := len(rows) - 1; i >= 0; i-- {
			if strings.Contains(rows[i], stub) {
				return i
			}
		}
		return -1
	}
	for i, row := range rows {
		if strings.Contains(row, stub) {
			return i
		}
	}
	return -1
}
