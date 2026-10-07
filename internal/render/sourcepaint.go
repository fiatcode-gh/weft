package render

import (
	"bytes"
	"image/color"
	"strings"
	"sync"

	gansi "charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gtext "github.com/yuin/goldmark/text"
)

// sourceMarkdown parses with the extensions Glamour's renderer uses, so the
// inline structure of a source line is the one the read view draws.
var sourceMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList))

// cascade is Glamour's cascadeStylePrimitive (ansi/style.go, v2.0.1) with
// toBlock=false, the form the colour of every element goes through: child's
// set fields replace parent's, unset ones inherit. Only the text attributes
// matter to the editor; the prefix, suffix and format fields stay the child's.
func cascade(parent, child gansi.StylePrimitive) gansi.StylePrimitive {
	s := child

	s.Color = parent.Color
	s.BackgroundColor = parent.BackgroundColor
	s.Underline = parent.Underline
	s.Bold = parent.Bold
	s.Upper = parent.Upper
	s.Title = parent.Title
	s.Lower = parent.Lower
	s.Italic = parent.Italic
	s.CrossedOut = parent.CrossedOut
	s.Faint = parent.Faint
	s.Conceal = parent.Conceal
	s.Inverse = parent.Inverse
	s.Blink = parent.Blink

	if child.Color != nil {
		s.Color = child.Color
	}
	if child.BackgroundColor != nil {
		s.BackgroundColor = child.BackgroundColor
	}
	if child.Underline != nil {
		s.Underline = child.Underline
	}
	if child.Bold != nil {
		s.Bold = child.Bold
	}
	if child.Upper != nil {
		s.Upper = child.Upper
	}
	if child.Lower != nil {
		s.Lower = child.Lower
	}
	if child.Title != nil {
		s.Title = child.Title
	}
	if child.Italic != nil {
		s.Italic = child.Italic
	}
	if child.CrossedOut != nil {
		s.CrossedOut = child.CrossedOut
	}
	if child.Faint != nil {
		s.Faint = child.Faint
	}
	if child.Conceal != nil {
		s.Conceal = child.Conceal
	}
	if child.Inverse != nil {
		s.Inverse = child.Inverse
	}
	if child.Blink != nil {
		s.Blink = child.Blink
	}
	return s
}

// toUV converts a resolved style the way Glamour's renderText draws it:
// colours through lipgloss.Color, then bold, italic, strikethrough, reverse,
// blink and a single underline. Faint, conceal and the case transforms are
// ignored, as Glamour ignores them.
func toUV(sp gansi.StylePrimitive) uv.Style {
	var s uv.Style
	if sp.Color != nil {
		s.Fg = realColor(lipgloss.Color(*sp.Color))
	}
	if sp.BackgroundColor != nil {
		s.Bg = realColor(lipgloss.Color(*sp.BackgroundColor))
	}
	if on(sp.Underline) {
		s.Underline = uv.UnderlineStyleSingle
	}
	if on(sp.Bold) {
		s.Attrs |= uv.AttrBold
	}
	if on(sp.Italic) {
		s.Attrs |= uv.AttrItalic
	}
	if on(sp.CrossedOut) {
		s.Attrs |= uv.AttrStrikethrough
	}
	if on(sp.Inverse) {
		s.Attrs |= uv.AttrReverse
	}
	if on(sp.Blink) {
		s.Attrs |= uv.AttrBlink
	}
	return s
}

func on(b *bool) bool { return b != nil && *b }

// realColor maps lipgloss's "no colour" to an unset colour.
func realColor(c color.Color) color.Color {
	if _, none := c.(lipgloss.NoColor); none {
		return nil
	}
	return c
}

// applyOverlay sets the fields lipgloss style l sets over s; unset fields keep
// s's value. This is what a terminal does with an SGR emitted without a reset.
func applyOverlay(s uv.Style, l lipgloss.Style) uv.Style {
	if c := realColor(l.GetForeground()); c != nil {
		s.Fg = c
	}
	if c := realColor(l.GetBackground()); c != nil {
		s.Bg = c
	}
	if l.GetBold() {
		s.Attrs |= uv.AttrBold
	}
	if l.GetItalic() {
		s.Attrs |= uv.AttrItalic
	}
	if l.GetUnderline() {
		s.Underline = uv.UnderlineStyleSingle
	}
	if l.GetStrikethrough() {
		s.Attrs |= uv.AttrStrikethrough
	}
	if l.GetReverse() {
		s.Attrs |= uv.AttrReverse
	}
	if l.GetBlink() {
		s.Attrs |= uv.AttrBlink
	}
	return s
}

// Painter colours source lines from a Theme, element by element as the read
// view colours the same text, with the syntax the read view hides dimmed.
type Painter struct {
	theme Theme
	// Block bases, resolved once: what bs.Current().Style is for the block a
	// line sits in.
	doc, paragraph, list, quote, quotePara gansi.StylePrimitive

	// highlight is quick.Highlight's output for code; tests replace it.
	highlight func(code, lang string) (string, error)
	fences    fenceCache
}

var chromaRegistered sync.Once

// NewPainter prepares a painter for t. When t highlights code it makes
// Glamour register its "charm" chroma style first (the read view does that on
// its first fence), so the painter's highlight finds it.
func NewPainter(t Theme) *Painter {
	c := t.Config
	p := &Painter{theme: t}
	p.doc = c.Document.StylePrimitive
	p.paragraph = cascade(p.doc, c.Paragraph.StylePrimitive)
	p.list = cascade(p.doc, c.List.StylePrimitive)
	p.quote = cascade(p.doc, c.BlockQuote.StylePrimitive)
	p.quotePara = cascade(p.quote, c.Paragraph.StylePrimitive)
	p.highlight = p.quickHighlight
	if c.CodeBlock.Chroma != nil {
		chromaRegistered.Do(func() {
			if r, err := rendererFor(80); err == nil {
				_, _ = r.Render("```\nx\n```\n")
			}
		})
	}
	return p
}

// dimmed is st with Faint added: the one thing the editor draws that the read
// view does not.
func dimmed(st uv.Style) uv.Style {
	st.Attrs |= uv.AttrFaint
	return st
}

// lineBuf is the per-byte style of one line while it is painted. cov marks the
// bytes a style has been chosen for.
type lineBuf struct {
	line, masked string
	st           []uv.Style
	cov          []bool
}

func (b *lineBuf) fill(start, end int, st uv.Style) {
	for i := start; i < end; i++ {
		b.st[i], b.cov[i] = st, true
	}
}

// Paint returns the styled spans of source line i: sorted, non-overlapping,
// bytes without a style left out.
func (p *Painter) Paint(src SourceLines, i int, info LineInfo, sc *Scanner) []Span {
	line := src.Line(i)
	if line == "" {
		return nil
	}
	b := &lineBuf{line: line, masked: line, st: make([]uv.Style, len(line)), cov: make([]bool, len(line))}
	var wikis []wikiLink
	switch info.Kind {
	case KindHidden, KindFence, KindSetextUnderline:
		b.fill(0, len(line), dimmed(toUV(p.doc)))
	case KindCode:
		p.paintCode(b, src, sc, i, info)
	case KindRule:
		b.fill(0, len(line), toUV(cascade(p.doc, p.theme.Config.HorizontalRule)))
	case KindBlank:
	default:
		b.masked, wikis = maskWikiLinks(line)
		p.paintBlock(b, src, sc, i, info)
	}
	for _, w := range wikis {
		for k := w.start; k < w.textStart; k++ {
			b.st[k] = dimmed(b.st[k])
		}
		for k := w.textEnd; k < w.end; k++ {
			b.st[k] = dimmed(b.st[k])
		}
		for k := w.textStart; k < w.textEnd; k++ {
			b.st[k] = applyOverlay(b.st[k], p.theme.Link)
		}
	}
	if info.Kind == KindBullet {
		if m := taskMarkerRe.FindStringSubmatchIndex(line); m != nil {
			if ms, ok := p.theme.Markers[line[m[4]:m[5]]]; ok {
				for k := m[4]; k < m[5]; k++ {
					b.st[k] = applyOverlay(b.st[k], ms)
				}
			}
		}
	}
	return b.spans()
}

func (b *lineBuf) spans() []Span {
	var spans []Span
	for i := 0; i < len(b.st); {
		if b.st[i].IsZero() {
			i++
			continue
		}
		j := i + 1
		for j < len(b.st) && b.st[j].Equal(&b.st[i]) {
			j++
		}
		spans = append(spans, Span{i, j, b.st[i]})
		i = j
	}
	return spans
}

// paintBlock paints a line of a text block: paragraph, heading, list item,
// quote or table row.
func (p *Painter) paintBlock(b *lineBuf, src SourceLines, sc *Scanner, i int, info LineInfo) {
	cfg := p.theme.Config
	switch info.Kind {
	case KindBullet:
		b.fill(0, info.Marker, toUV(cascade(p.list, cfg.Item)))
		p.paintLeaf(b, info.Marker, len(b.line), leaf{parent: p.list, para: p.list})
	case KindOrdered:
		b.fill(0, info.Marker, toUV(cascade(p.list, cfg.Enumeration)))
		p.paintLeaf(b, info.Marker, len(b.line), leaf{parent: p.list, para: p.list})
	case KindQuote:
		b.fill(0, info.Marker, toUV(p.quote))
		p.paintLeaf(b, info.Marker, len(b.line), leaf{parent: p.quote, para: p.quotePara})
	case KindHeading:
		p.paintLeaf(b, 0, len(b.line), leaf{parent: p.doc, para: p.paragraph})
	case KindSetextHeading:
		p.paintLeaf(b, info.Marker, len(b.line), leaf{parent: p.doc, para: p.paragraph, level: info.Level})
	case KindTable:
		p.paintTable(b, src, sc, i)
	default: // KindText
		if info.InList {
			p.paintLeaf(b, info.Marker, len(b.line), leaf{parent: p.list, para: p.list})
		} else {
			p.paintLeaf(b, info.Marker, len(b.line), leaf{parent: p.doc, para: p.paragraph})
		}
	}
}

// paintTable paints the cells of a table row; pipes and the delimiter row stay
// unstyled (lipgloss table borders carry no colour).
func (p *Painter) paintTable(b *lineBuf, src SourceLines, sc *Scanner, i int) {
	if i > 0 && tableDelimRe.MatchString(b.line) && sc.Info(src, i-1).Kind == KindTable &&
		(i == 1 || sc.Info(src, i-2).Kind != KindTable) {
		return
	}
	table := p.theme.Config.Table.StylePrimitive
	start := 0
	for k := 0; k <= len(b.line); k++ {
		if k < len(b.line) && (b.line[k] != '|' || k > 0 && b.line[k-1] == '\\') {
			continue
		}
		p.paintLeaf(b, start, k, leaf{parent: p.doc, para: p.doc, ov: &table, inline: true})
		start = k + 1
	}
}

// leaf says how one leaf block's text is styled.
type leaf struct {
	parent gansi.StylePrimitive  // the block the leaf sits in; headings cascade from it
	para   gansi.StylePrimitive  // base of a paragraph
	level  int                   // heading level forced on a paragraph (setext)
	ov     *gansi.StylePrimitive // style every text child is overridden with (table cells)
	inline bool                  // only inline content: no heading, no other block
}

// paintLeaf parses line[start:end) as a tiny markdown document and styles its
// inline structure. Bytes no Text, code-span content or autolink covers are the
// syntax the read view hides and get dimmed.
func (p *Painter) paintLeaf(b *lineBuf, start, end int, o leaf) {
	text := b.masked[start:end]
	if strings.TrimSpace(text) == "" {
		return
	}
	src := []byte(text)
	doc := sourceMarkdown.Parser().Parse(gtext.NewReader(src))
	first := doc.FirstChild()
	base := o.para
	open, openLen := 0, 0
	switch n := first.(type) {
	case nil:
	case *ast.Heading:
		if o.inline {
			p.fillText(b, start, end, o.para)
			return
		}
		base = cascade(o.parent, p.headingRules(n.Level))
		open, openLen = len(text)-len(strings.TrimLeft(text, " ")), n.Level
	case *ast.Paragraph, *ast.TextBlock:
		if o.level > 0 {
			base = cascade(o.parent, p.headingRules(o.level))
		}
	default:
		p.fillText(b, start, end, o.para)
		return
	}
	if first != nil {
		w := &inliner{p: p, b: b, off: start, src: src, base: base}
		for c := first.FirstChild(); c != nil; c = c.NextSibling() {
			w.node(c, o.ov)
		}
	}
	b.fill(start+open, start+open+openLen, toUV(base))
	plain := toUV(cascade(base, p.theme.Config.Text))
	faint := dimmed(plain)
	for k := start; k < end; k++ {
		if b.cov[k] {
			continue
		}
		if c := b.line[k]; c == ' ' || c == '\t' {
			b.st[k], b.cov[k] = plain, true
		} else {
			b.st[k], b.cov[k] = faint, true
		}
	}
}

// fillText styles [start, end) as plain text of base.
func (p *Painter) fillText(b *lineBuf, start, end int, base gansi.StylePrimitive) {
	b.fill(start, end, toUV(cascade(base, p.theme.Config.Text)))
}

// headingRules is cascadeStyles(Heading, Hn) as HeadingElement builds it.
func (p *Painter) headingRules(level int) gansi.StylePrimitive {
	c := p.theme.Config
	h := c.H6
	switch level {
	case 1:
		h = c.H1
	case 2:
		h = c.H2
	case 3:
		h = c.H3
	case 4:
		h = c.H4
	case 5:
		h = c.H5
	}
	return cascade(c.Heading.StylePrimitive, h.StylePrimitive)
}

// inliner walks the inline nodes of one leaf block and styles what each of
// them covers, the way Glamour's elements draw them.
type inliner struct {
	p    *Painter
	b    *lineBuf
	off  int // offset of src within the line
	src  []byte
	base gansi.StylePrimitive
	pos  int // end of the last styled range, where an autolink search starts
}

func (w *inliner) paint(start, stop int, st gansi.StylePrimitive) {
	if start < 0 || stop > len(w.src) || start >= stop {
		return
	}
	w.b.fill(w.off+start, w.off+stop, toUV(st))
	w.pos = stop
}

// texts returns the segments of every Text node under n.
func texts(n ast.Node) []gtext.Segment {
	var segs []gtext.Segment
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			segs = append(segs, t.Segment)
		} else {
			segs = append(segs, texts(c)...)
		}
	}
	return segs
}

func (w *inliner) paintTexts(n ast.Node, st gansi.StylePrimitive) {
	for _, s := range texts(n) {
		w.paint(s.Start, s.Stop, st)
	}
}

// node styles n. ov is the style an enclosing emphasis, link or table cell
// passes down to children that accept one (Text, Strikethrough, emphasis);
// links, code spans, autolinks and images ignore it, as in Glamour.
func (w *inliner) node(n ast.Node, ov *gansi.StylePrimitive) {
	cfg := w.p.theme.Config
	over := func(st gansi.StylePrimitive) gansi.StylePrimitive {
		if ov != nil {
			return cascade(st, *ov)
		}
		return st
	}
	switch n := n.(type) {
	case *ast.Text:
		w.paint(n.Segment.Start, n.Segment.Stop, over(cascade(w.base, cfg.Text)))
		w.dimEscapes(n.Segment)
	case *ast.Emphasis:
		style := cfg.Emph
		if n.Level > 1 {
			style = cfg.Strong
		}
		style = over(style)
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			w.node(c, &style)
		}
	case *extast.Strikethrough:
		w.paintTexts(n, over(cascade(w.base, cfg.Strikethrough)))
	case *ast.CodeSpan:
		w.paintTexts(n, cascade(w.base, cfg.Code.StylePrimitive))
	case *ast.Link:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			w.node(c, &cfg.LinkText)
		}
	case *ast.AutoLink:
		if i := bytes.Index(w.src[w.pos:], n.Label(w.src)); i >= 0 {
			s := w.pos + i
			w.paint(s, s+len(n.Label(w.src)), cascade(w.base, cfg.Link))
		}
	case *ast.Image:
		w.paintTexts(n, cascade(w.base, cfg.ImageText))
	case *ast.RawHTML:
		for k := 0; k < n.Segments.Len(); k++ {
			s := n.Segments.At(k)
			w.paint(s.Start, s.Stop, cascade(w.base, cfg.HTMLSpan.StylePrimitive))
		}
	default:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			w.node(c, ov)
		}
	}
}

// wikiLink is one [[...]] in a line: its bytes [start, end) and the display
// text [textStart, textEnd), the alias when there is one, else the target.
type wikiLink struct{ start, end, textStart, textEnd int }

// maskWikiLinks replaces every wiki-link outside code spans with the same
// number of 'x' bytes, a word character like the read view's sentinel, so
// goldmark's emphasis flanking sees what the read view's Glamour sees. Code
// spans are found by the read view's rule: odd parts of a split on backticks.
func maskWikiLinks(line string) (string, []wikiLink) {
	if !strings.Contains(line, "[[") {
		return line, nil
	}
	masked := []byte(line)
	var links []wikiLink
	off := 0
	for k, part := range strings.Split(line, "`") {
		if k%2 == 0 {
			for _, m := range wikiLinkRe.FindAllStringSubmatchIndex(part, -1) {
				w := wikiLink{off + m[0], off + m[1], off + m[2], off + m[3]}
				if m[5] > m[4] {
					w.textStart, w.textEnd = off+m[4], off+m[5]
				}
				for q := w.start; q < w.end; q++ {
					masked[q] = 'x'
				}
				links = append(links, w)
			}
		}
		off += len(part) + 1
	}
	return string(masked), links
}

// escapable is the set of bytes Glamour's escapeReplacer unescapes: a
// backslash before one of them is not drawn.
const escapable = "\\`*_{}[]<>()#+-.!|"

// dimEscapes dims the backslashes of escapes in a Text segment, which
// goldmark keeps in the text and Glamour drops.
func (w *inliner) dimEscapes(seg gtext.Segment) {
	faint := dimmed(toUV(cascade(w.base, w.p.theme.Config.Text)))
	for k := seg.Start; k+1 < seg.Stop; k++ {
		if w.src[k] == '\\' && strings.IndexByte(escapable, w.src[k+1]) >= 0 {
			w.b.st[w.off+k] = faint
			k++
		}
	}
}
