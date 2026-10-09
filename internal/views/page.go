package views

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// leadingTrimmedLines is how many whole lines load's strings.TrimSpace drops
// from the top of raw: the offset from a line of the rendered body to the
// same line in the file.
func leadingTrimmedLines(raw string) int {
	return strings.Count(raw[:len(raw)-len(strings.TrimLeftFunc(raw, unicode.IsSpace))], "\n")
}

// renderCount is incremented on every renderPage call inside
// PageView. Tests assert that the per-page cache keeps it from
// growing when re-rendering unchanged pages.
var renderCount int64

// RenderCount returns the current value of the per-page render
// counter. Used by tests.
func RenderCount() int64 { return renderCount }

// renderPage is the single entry into the render package — a var so tests
// can inject render results. RenderWithEmphasis("") is identical to Render.
var renderPage = render.RenderWithEmphasis

// sourceRowsFor builds the row map behind ReadingAnchor — a var so tests
// can count calls or inject a missing map.
var sourceRowsFor = render.SourceRows

// PageView renders a single page with a row cursor and a wiki-link cursor.
// The row cursor (row) is the line the reader is on; top is the first row on
// screen. The link cursor (cursor) is a separate selection among the page's
// links.
type PageView struct {
	idx      *graph.Index
	page     string
	result   render.Result
	vp       viewport.Model // window renderer only: View feeds it the rows on screen
	cursor   int            // index into result.Links, or -1
	emphasis string         // transient term to highlight on arrival; "" = none
	err      error
	width    int
	height   int
	cache    map[string]cachedPage

	top      int      // first row on screen
	row      int      // cursor row, always a non-blank row; -1 when the page has none
	full     []string // result.Styled split into rows
	rowStart []int    // byte offset of each row of full in result.Styled

	body     string // body result was rendered from; "" when nothing rendered
	rows     []int  // memoised render.SourceRows(body, ...), valid when rowsDone
	rowOwn   []bool // rows[i]'s row is one of its line's own, not borrowed
	rowsDone bool
	noColor  bool // the style names no colours, so the link cursor needs reverse video
}

type cachedPage struct {
	result  render.Result
	body    string
	modTime time.Time
	width   int
}

func NewPageView(idx *graph.Index, page string, width, height int) *PageView {
	pv := &PageView{
		idx:    idx,
		page:   page,
		width:  width,
		height: height,
		cursor: -1,
		cache:  map[string]cachedPage{},
		vp:     viewport.New(viewport.WithWidth(width), viewport.WithHeight(max(1, height-2))),
	}
	pv.load()
	pv.row = pv.snap(0, +1)
	return pv
}

// Page returns the currently displayed page name.
func (p *PageView) Page() string { return p.page }

// SetPage switches to a different page in the same index. The view starts at
// the top — a fresh navigation must not inherit the previous page's scroll
// position (history Restore sets an explicit position after this when walking
// [ / ]).
func (p *PageView) SetPage(name string) {
	p.page = name
	p.cursor = -1
	p.emphasis = ""
	p.load()
	p.top, p.row = 0, p.snap(0, +1)
}

// SetPageEmphasizing switches to name and highlights whole-word occurrences of
// term (the page navigated from), scrolling to the first. The highlight is
// transient: a later SetPage or history Restore clears it.
func (p *PageView) SetPageEmphasizing(name, term string) {
	p.page = name
	p.cursor = -1
	p.emphasis = term
	p.load()
	p.top, p.row = 0, p.snap(0, +1)
	p.scrollToFirstFind()
}

// SetSize updates the window size. The page body is only re-rendered when the
// width changes — height changes don't affect word-wrap, so the existing rows
// are just re-clipped.
func (p *PageView) SetSize(w, h int) {
	widthChanged := w != p.width
	p.width, p.height = w, h
	p.vp.SetWidth(w)
	p.vp.SetHeight(max(1, h-2))
	if widthChanged {
		p.load()
	}
	p.Reposition(p.top, p.row)
}

// rowCount is how many rows the page has.
func (p *PageView) rowCount() int { return len(p.full) }

// rowText is row i of the page, styled.
func (p *PageView) rowText(i int) string { return p.full[i] }

// indexRows splits the rendered page into rows and records where each starts.
func (p *PageView) indexRows() {
	s := p.result.Styled
	if s == "" {
		p.full, p.rowStart = nil, nil
		return
	}
	p.full = strings.Split(s, "\n")
	p.rowStart = make([]int, len(p.full))
	off := 0
	for i, r := range p.full {
		p.rowStart[i] = off
		off += len(r) + 1
	}
}

// rowOf is the row holding byte offset off of the rendered page.
func (p *PageView) rowOf(off int) int {
	return max(0, sort.SearchInts(p.rowStart, off+1)-1)
}

func (p *PageView) rowBlank(i int) bool { return !nonBlank(p.rowText(i)) }

// maxTop is the largest top that still fills the window.
func (p *PageView) maxTop() int { return max(0, p.rowCount()-p.vp.Height()) }

// next is the nearest non-blank row from i in direction dir (exclusive), -1
// when there is none.
func (p *PageView) next(i, dir int) int {
	for j := i + dir; j >= 0 && j < p.rowCount(); j += dir {
		if !p.rowBlank(j) {
			return j
		}
	}
	return -1
}

// snap is the nearest non-blank row at or beyond r in direction dir, else
// the nearest on the other side; -1 when the page has none.
func (p *PageView) snap(r, dir int) int {
	n := p.rowCount()
	if n == 0 {
		return -1
	}
	r = clampInt(r, 0, n-1)
	if !p.rowBlank(r) {
		return r
	}
	if j := p.next(r, dir); j >= 0 {
		return j
	}
	return p.next(r, -dir)
}

// snapIn is snap confined to rows [lo, hi]: at or after r, else before.
func (p *PageView) snapIn(r, lo, hi int) int {
	for j := r; j <= hi; j++ {
		if !p.rowBlank(j) {
			return j
		}
	}
	for j := r - 1; j >= lo; j-- {
		if !p.rowBlank(j) {
			return j
		}
	}
	return -1
}

// follow scrolls the least that keeps the cursor row on screen.
func (p *PageView) follow() {
	if p.row >= 0 {
		p.top = min(p.top, p.row)
		p.top = max(p.top, p.row-p.vp.Height()+1)
	}
	p.top = clampInt(p.top, 0, p.maxTop())
}

// centreOn puts row r in the middle of the window and the cursor on it.
func (p *PageView) centreOn(r int) {
	p.row = p.snap(r, +1)
	p.top = clampInt(r-p.vp.Height()/2, 0, p.maxTop())
	p.follow()
}

// CycleLink moves the link cursor to the next/previous wiki-link and the row
// cursor onto its row, scrolling so the new cursor target is visible.
// dir=+1 forwards, dir=-1 backwards.
func (p *PageView) CycleLink(dir int) {
	if len(p.result.Links) == 0 {
		return
	}
	if p.cursor == -1 {
		if dir > 0 {
			p.cursor = 0
		} else {
			p.cursor = len(p.result.Links) - 1
		}
	} else {
		p.cursor = (p.cursor + dir + len(p.result.Links)) % len(p.result.Links)
	}
	p.scrollToCursor()
}

// scrollToCursor puts the row cursor on the link at p.cursor and scrolls so it
// is inside the visible window. Links already in view leave the top
// untouched; off-screen links are centred vertically in the window.
func (p *PageView) scrollToCursor() {
	row, ok := p.linkRow(p.cursor)
	if !ok {
		return
	}
	p.row = row
	if row >= p.top && row <= p.top+p.vp.Height()-1 {
		return
	}
	p.top = clampInt(row-p.vp.Height()/2, 0, p.maxTop())
}

// linkRow returns the styled row of link i, or false when i or the link's
// offset is out of range. Shared by scrollToCursor and View so both agree on
// where a link sits.
func (p *PageView) linkRow(i int) (int, bool) {
	if i < 0 || i >= len(p.result.Links) {
		return 0, false
	}
	l := p.result.Links[i]
	if l.Start < 0 || l.Start > len(p.result.Styled) {
		return 0, false
	}
	return p.rowOf(l.Start), true
}

// Anchor says where the in-app editor opens: row RowInLine of body line Line
// (a row of the line as the editor wraps it) goes on screen row ScreenRow.
type Anchor struct{ Line, RowInLine, ScreenRow int }

// ReadingAnchor reports the row of the rendered page body (the TrimSpace'd body
// load renders) the in-app editor should keep in place: the cursor row's line,
// on the screen row the cursor is on. A row no source line spells (a table's
// outer border, a margin) never stands for the line below it: the first
// non-blank own row at or after the cursor decides, else the last one before
// it. ok is false when a row map is needed and none can be produced. A view
// at the top with the cursor on the first non-blank row needs no map: that
// row is line 0. The map is computed lazily, once per load, and only after an
// anchor is known to need it.
func (p *PageView) ReadingAnchor() (Anchor, bool) {
	if p.row < 0 {
		return Anchor{}, true
	}
	if p.top == 0 && p.row == p.snap(0, +1) {
		return Anchor{0, 0, p.row}, true
	}
	lines := p.sourceRows()
	if len(lines) == 0 {
		return Anchor{}, false
	}
	n := min(len(lines), p.rowCount())
	a := min(p.row, n-1)
	own := func(i int) bool { return !p.rowBlank(i) && p.rowOwned(i) }
	found := false
	for i := a; i < n && !found; i++ {
		if own(i) {
			a, found = i, true
		}
	}
	for i := min(p.row, n-1) - 1; i >= 0 && !found; i-- {
		if own(i) {
			a, found = i, true
		}
	}
	line := lines[a]
	f, owned := a, p.rowOwned(a)
	for r := a - 1; r >= 0 && lines[r] == line; r-- {
		if p.rowBlank(r) {
			continue
		}
		if p.rowOwned(r) || !owned {
			f, owned = r, p.rowOwned(r)
		}
	}
	return Anchor{Line: line, RowInLine: a - f, ScreenRow: a - p.top}, true
}

// PlaceAnchor scrolls so the row RowInLine of body line a.Line sits on screen
// row a.ScreenRow, the inverse of ReadingAnchor, and puts the cursor on it. A
// line the read view hides has no rows: the nearest later line with rows
// stands in, else the nearest earlier one. RowInLine clamps to the line's last
// row, ScreenRow to the last row of the page window, and the top to the page.
// The editor's window is the page's, so the ScreenRow clamp only guards a
// terminal too small for the two to agree. The link cursor is kept.
func (p *PageView) PlaceAnchor(a Anchor) {
	lines := p.sourceRows()
	if len(lines) == 0 {
		return
	}
	first := map[int]int{} // body line → first non-blank row of its own
	borrowed := map[int]int{}
	for r := range min(len(lines), p.rowCount()) {
		if p.rowBlank(r) {
			continue
		}
		set := first
		if !p.rowOwned(r) {
			set = borrowed
		}
		if _, seen := set[lines[r]]; !seen {
			set[lines[r]] = r
		}
	}
	for l, r := range borrowed { // a line with only borrowed rows still has rows
		if _, own := first[l]; !own {
			first[l] = r
		}
	}
	line, found := 0, false
	for l := range first {
		if l >= a.Line && (!found || l < line) {
			line, found = l, true
		}
	}
	if !found {
		for l := range first {
			if !found || l > line {
				line, found = l, true
			}
		}
	}
	if !found {
		return
	}
	f := first[line]
	n := 0
	for r := f; r < len(lines) && lines[r] == line; r++ {
		n++
	}
	t := f + min(max(a.RowInLine, 0), n-1)
	p.row = t
	p.top = clampInt(t-min(a.ScreenRow, p.vp.Height()-1), 0, p.maxTop())
}

func nonBlank(row string) bool { return strings.TrimSpace(ansi.Strip(row)) != "" }

// sourceRows returns the styled-row → body-line map for the loaded render,
// computing it on first use after each load. rowOwned says whether a row is
// one of its line's own (a letterless margin or border row only borrows the
// line below it).
func (p *PageView) sourceRows() []int {
	if !p.rowsDone {
		p.rowsDone = true
		if p.body != "" {
			p.rows, p.rowOwn = sourceRowsFor(p.body, p.width, p.emphasis)
		}
	}
	return p.rows
}

func (p *PageView) rowOwned(row int) bool {
	return row < 0 || row >= len(p.rowOwn) || p.rowOwn[row]
}

// FollowCursor returns the link target under the cursor, or "" if none.
func (p *PageView) FollowCursor() string {
	if p.cursor < 0 || p.cursor >= len(p.result.Links) {
		return ""
	}
	return p.result.Links[p.cursor].Target
}

// FocusLinkTo positions the link cursor on the first link whose target resolves
// to name and scrolls it into view. No-op when the page has no such link.
func (p *PageView) FocusLinkTo(name string) {
	for i, l := range p.result.Links {
		if resolved, ok := p.idx.Resolve(l.Target); ok && resolved.Name == name {
			p.cursor = i
			p.scrollToCursor()
			return
		}
	}
}

// LineDown/LineUp move the row cursor to the next/previous non-blank row; the
// view scrolls only when the cursor would leave the window. No-op at the ends.
func (p *PageView) LineDown() { p.step(+1) }
func (p *PageView) LineUp()   { p.step(-1) }

func (p *PageView) step(dir int) {
	if p.row < 0 {
		return
	}
	if j := p.next(p.row, dir); j >= 0 {
		p.row = j
		p.follow()
	}
}

// HalfPageDown/HalfPageUp move the top and the cursor row by half a window.
func (p *PageView) HalfPageDown() { p.half(+1) }
func (p *PageView) HalfPageUp()   { p.half(-1) }

func (p *PageView) half(dir int) {
	if p.row < 0 {
		return
	}
	d := max(1, p.vp.Height()/2) * dir
	p.top = clampInt(p.top+d, 0, p.maxTop())
	p.row = p.snap(clampInt(p.row+d, 0, p.rowCount()-1), dir)
	p.follow()
}

// GotoTop puts the cursor on the first non-blank row, GotoBottom on the last.
func (p *PageView) GotoTop() {
	p.top, p.row = 0, p.snap(0, +1)
}

func (p *PageView) GotoBottom() {
	if p.row < 0 {
		return
	}
	p.row = p.snap(p.rowCount()-1, -1)
	p.top = p.maxTop()
	p.follow()
}

// Offset returns the first row on screen.
func (p *PageView) Offset() int { return p.top }

// CursorRow returns the cursor's row, -1 when the page has no non-blank row.
func (p *PageView) CursorRow() int { return p.row }

// Cursor returns the current link cursor index. -1 means no link selected.
func (p *PageView) Cursor() int { return p.cursor }

// ScrollIndicator returns "" when the page fits the window (no scroll
// possible), otherwise "NN%" — 0% at the top, 100% at the bottom.
func (p *PageView) ScrollIndicator() string {
	n, h := p.rowCount(), p.vp.Height()
	if n <= h {
		return ""
	}
	pct := min(1, max(0, float64(p.top)/float64(n-h)))
	return fmt.Sprintf("%d%%", int(pct*100))
}

// Restore sets the scroll position, cursor row and link cursor in one shot:
// see Reposition. The link cursor is clamped to a valid link index; anything
// outside [0, len(Links)) falls back to -1 (no link selected). Use after
// SetPage to recover state captured before a navigation.
func (p *PageView) Restore(offset, row, cursor int) {
	p.Reposition(offset, row)
	if cursor < 0 || cursor >= len(p.result.Links) {
		cursor = -1
	}
	p.cursor = cursor
}

// Reposition sets the top (clamped to the page) and the cursor row: snapped to
// the nearest non-blank row at or after row (else before), then moved into the
// window without moving the top. The link cursor is untouched.
func (p *PageView) Reposition(top, row int) {
	n := p.rowCount()
	p.top = clampInt(top, 0, p.maxTop())
	if n == 0 {
		p.row = -1
		return
	}
	hi := min(n-1, p.top+p.vp.Height()-1)
	r := p.snap(max(row, 0), +1)
	if r >= 0 && (r < p.top || r > hi) {
		r = clampInt(r, p.top, hi)
		if p.rowBlank(r) {
			if j := p.snapIn(r, p.top, hi); j >= 0 {
				r = j
			} else {
				r = p.snap(r, +1)
			}
		}
	}
	p.row = r
	p.follow()
}

// ScrollToTask centres the view on the open todo at the given 0-based
// ordinal (its position among the page's open todos in document order, as
// recorded in render.Result.Tasks) and puts the cursor on it. No-op when the
// ordinal is out of range. One-shot deep-link jump applied at SetPage time —
// not a property Restore rewinds into.
func (p *PageView) ScrollToTask(ordinal int) {
	if ordinal < 0 || ordinal >= len(p.result.Tasks) {
		return
	}
	off := p.result.Tasks[ordinal]
	if off < 0 || off > len(p.result.Styled) {
		return
	}
	p.centreOn(p.rowOf(off))
}

// cursorStyle: bright background + dark foreground + bold + underline so the
// cursored link still reads as a link (underline) while standing out from the
// other links on the page. Rendered in a single pass over the link's display
// text — never nested over the pre-styled bytes from render.Render, which
// would emit overlapping SGR resets.
var cursorStyle = lipgloss.NewStyle().
	Background(colorCursor). // bright yellow
	Foreground(colorSelFg).  // black
	Bold(true).
	Underline(true)

// StatusLine returns the text to display on the left side of the App-level
// status bar: page name, plus a link count or cursor position when relevant.
func (p *PageView) StatusLine() string {
	out := styleTitle.Render(p.page)
	if n := len(p.result.Links); n > 0 {
		var meta string
		if p.cursor >= 0 {
			meta = fmt.Sprintf("  ·  link %d/%d", p.cursor+1, n)
		} else {
			meta = fmt.Sprintf("  ·  %d links", n)
		}
		out += styleFaint.Render(meta)
	}
	return out
}

// View draws the rows on screen: the page's own bytes, except the cursor row
// (its plain text in the selection style) and the selected link (in the link
// cursor style). Only the window is handed to the viewport, so a frame costs
// the window, not the page.
func (p *PageView) View() string {
	if p.err != nil {
		return fmt.Sprintf("error: %v", p.err)
	}
	if p.rowCount() == 0 {
		p.vp.SetContent(styleFaint.Render("(no entry yet for this page)"))
		return p.vp.View()
	}
	lo, hi := p.top, min(p.rowCount(), p.top+p.vp.Height())
	window := make([]string, hi-lo)
	copy(window, p.full[lo:hi])

	var link *render.Link
	linkRow := -1
	if p.cursor >= 0 && p.cursor < len(p.result.Links) {
		link = &p.result.Links[p.cursor]
		linkRow = p.rowOf(link.Start)
	}
	cs := cursorStyle
	if p.noColor {
		// No colour to tell the cursor link apart: reverse video does.
		cs = cs.Reverse(true)
	}
	if linkRow >= lo && linkRow < hi && linkRow != p.row {
		row, rs := p.full[linkRow], p.rowStart[linkRow]
		window[linkRow-lo] = row[:link.Start-rs] + cs.Render(link.Display) + row[min(link.End-rs, len(row)):]
	}
	if p.row >= lo && p.row < hi {
		window[p.row-lo] = p.cursorRow(link, linkRow, cs)
	}
	p.vp.SetContentLines(window)
	return p.vp.View()
}

// cursorRow draws the cursor row: its plain text padded to the page width in
// the selection style, with the selected link, if it starts here, in cs.
func (p *PageView) cursorRow(link *render.Link, linkRow int, cs lipgloss.Style) string {
	st := styleSel
	if p.noColor {
		st = lipgloss.NewStyle().Bold(true).Reverse(true)
	}
	row := p.full[p.row]
	text := ansi.Strip(row)
	if pad := p.width - ansi.StringWidth(text); pad > 0 {
		text += strings.Repeat(" ", pad)
	}
	paint := func(s string) string {
		if s == "" {
			return ""
		}
		return st.Render(s)
	}
	if link == nil || linkRow != p.row {
		return paint(text)
	}
	col := ansi.StringWidth(row[:link.Start-p.rowStart[p.row]])
	end := col + ansi.StringWidth(link.Display)
	return paint(ansi.Cut(text, 0, col)) + cs.Render(link.Display) + paint(ansi.Cut(text, end, ansi.StringWidth(text)))
}

func (p *PageView) load() {
	p.loadResult()
	p.indexRows()
}

func (p *PageView) loadResult() {
	p.err = nil
	p.result = render.Result{}
	p.body, p.rows, p.rowOwn, p.rowsDone = "", nil, nil, false
	theme, _ := render.CurrentTheme()
	p.noColor = theme.Name == "notty"
	meta, ok := p.idx.Resolve(p.page)
	if !ok {
		return
	}
	// The per-page cache holds only plain (un-emphasised) renders. When an
	// emphasis term is set the render is transient — never read or write the
	// cache, so a later plain navigation can't be served a highlighted version.
	if p.emphasis == "" {
		if c, hit := p.cache[meta.Name]; hit && c.modTime.Equal(meta.ModTime) && c.width == p.width {
			p.result = c.result
			p.body = c.body
			return
		}
	}
	b, err := os.ReadFile(meta.Path)
	if err != nil {
		p.err = err
		return
	}
	atomic.AddInt64(&renderCount, 1)
	body := strings.TrimSpace(string(b)) + "\n"
	res, err := renderPage(body, p.width, p.emphasis)
	if err != nil {
		p.err = err
		return
	}
	if res.FallbackErr != nil {
		// Un-styled fallback: log the cause; the cache skip below keeps a
		// transient Glamour failure from sticking until the mtime changes.
		logRenderFallback(p.page, res.FallbackErr)
	}
	p.result = res
	p.body = body
	if p.emphasis == "" && res.FallbackErr == nil {
		p.cache[meta.Name] = cachedPage{result: res, body: body, modTime: meta.ModTime, width: p.width}
	}
}

// logRenderFallback best-effort appends a Glamour-fallback notice to
// DebugLogPath(). A write failure is dropped: the page already shows its
// readable raw-text fallback, and there is no status-bar in this layer to
// degrade to.
func logRenderFallback(page string, cause error) {
	f, err := os.OpenFile(DebugLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "render fallback for %q at %s: %v\n", page, time.Now().Format(time.RFC3339), cause)
}

// scrollToFirstFind centres the view on the first highlighted emphasis
// occurrence, if any, and puts the cursor on it.
func (p *PageView) scrollToFirstFind() {
	if len(p.result.Finds) == 0 {
		return
	}
	off := p.result.Finds[0]
	if off < 0 || off > len(p.result.Styled) {
		return
	}
	p.centreOn(p.rowOf(off))
}
