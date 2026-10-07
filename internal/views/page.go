package views

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

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

// sourceRowsFor builds the row map behind AnchorSourceLine — a var so tests
// can count calls or inject a missing map.
var sourceRowsFor = render.SourceRows

// PageView renders a single page with a wiki-link cursor.
type PageView struct {
	idx      *graph.Index
	page     string
	result   render.Result
	vp       viewport.Model
	cursor   int    // index into result.Links, or -1
	emphasis string // transient term to highlight on arrival; "" = none
	err      error
	width    int
	height   int
	cache    map[string]cachedPage

	body     string // body result was rendered from; "" when nothing rendered
	rows     []int  // memoised render.SourceRows(body, ...), valid when rowsDone
	rowsDone bool
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
	return pv
}

// Page returns the currently displayed page name.
func (p *PageView) Page() string { return p.page }

// SetPage switches to a different page in the same index. The viewport
// starts at the top — a fresh navigation must not inherit the previous
// page's scroll position (history Restore sets an explicit offset after
// this when walking [ / ]).
func (p *PageView) SetPage(name string) {
	p.page = name
	p.cursor = -1
	p.emphasis = ""
	p.load()
	p.vp.GotoTop()
}

// SetPageEmphasizing switches to name and highlights whole-word occurrences of
// term (the page navigated from), scrolling to the first. The highlight is
// transient: a later SetPage or history Restore clears it.
func (p *PageView) SetPageEmphasizing(name, term string) {
	p.page = name
	p.cursor = -1
	p.emphasis = term
	p.load()
	p.vp.GotoTop()
	p.scrollToFirstFind()
}

// SetSize updates viewport size. The page body is only re-rendered when the
// width changes — height changes don't affect word-wrap, so we just resize
// the viewport and let it re-clip the existing styled content.
func (p *PageView) SetSize(w, h int) {
	widthChanged := w != p.width
	p.width, p.height = w, h
	p.vp.SetWidth(w)
	p.vp.SetHeight(max(1, h-2))
	if widthChanged {
		p.load()
	}
}

// CycleLink moves the cursor to the next/previous wiki-link, scrolling the
// viewport so the new cursor target is visible.
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

// scrollToCursor adjusts the viewport's YOffset so the link at p.cursor is
// inside the visible window. Links already in view leave the offset
// untouched; off-screen links are centred vertically in the viewport.
func (p *PageView) scrollToCursor() {
	row, ok := p.linkRow(p.cursor)
	if !ok {
		return
	}
	top := p.vp.YOffset()
	bottom := top + p.vp.Height() - 1
	if row >= top && row <= bottom {
		return
	}
	target := row - p.vp.Height()/2
	if target < 0 {
		target = 0
	}
	p.vp.SetYOffset(target)
}

// linkRow returns the styled row of link i, or false when i or the link's
// offset is out of range. Shared by scrollToCursor and AnchorSourceLine so
// both agree on where a link sits.
func (p *PageView) linkRow(i int) (int, bool) {
	if i < 0 || i >= len(p.result.Links) {
		return 0, false
	}
	l := p.result.Links[i]
	if l.Start < 0 || l.Start > len(p.result.Styled) {
		return 0, false
	}
	return strings.Count(p.result.Styled[:l.Start], "\n"), true
}

// AnchorSourceLine reports the 0-based line of the rendered page body (the
// TrimSpace'd body load renders) that the in-app editor should open on.
// A link cursor whose row is inside the visible window wins; otherwise the
// top visible row decides. ok is false — open at the top of the file — when
// the view is at the top with no visible link cursor, or no row map can be
// produced. The map is computed lazily, once per load, and only after an
// anchor is known to be needed.
func (p *PageView) AnchorSourceLine() (line int, ok bool) {
	top := p.vp.YOffset()
	row := top
	if r, found := p.linkRow(p.cursor); found && r >= top && r <= top+p.vp.Height()-1 {
		row = r
	} else if top == 0 {
		return 0, false
	}
	rows := p.sourceRows()
	if len(rows) == 0 {
		return 0, false
	}
	return rows[clampInt(row, 0, len(rows)-1)], true
}

// sourceRows returns the styled-row → body-line map for the loaded render,
// computing it on first use after each load.
func (p *PageView) sourceRows() []int {
	if !p.rowsDone {
		p.rowsDone = true
		if p.body != "" {
			p.rows = sourceRowsFor(p.body, p.width, p.emphasis)
		}
	}
	return p.rows
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

// LineDown/LineUp/HalfPage/Goto delegates to viewport.
func (p *PageView) LineDown()     { p.vp.ScrollDown(1) }
func (p *PageView) LineUp()       { p.vp.ScrollUp(1) }
func (p *PageView) HalfPageDown() { p.vp.HalfPageDown() }
func (p *PageView) HalfPageUp()   { p.vp.HalfPageUp() }
func (p *PageView) GotoTop()      { p.vp.GotoTop() }
func (p *PageView) GotoBottom()   { p.vp.GotoBottom() }

// Offset returns the viewport's current scroll position (YOffset).
func (p *PageView) Offset() int { return p.vp.YOffset() }

// Cursor returns the current link cursor index. -1 means no link selected.
func (p *PageView) Cursor() int { return p.cursor }

// ScrollIndicator returns "" when the page fits the viewport (no scroll
// possible), otherwise "NN%" — 0% at the top, 100% at the bottom.
// viewport.ScrollPercent is already clamped to [0, 1] and returns exactly
// 0 at YOffset=0 and 1 at the max offset.
func (p *PageView) ScrollIndicator() string {
	if p.vp.TotalLineCount() <= p.vp.Height() {
		return ""
	}
	return fmt.Sprintf("%d%%", int(p.vp.ScrollPercent()*100))
}

// Restore sets the viewport scroll offset and link cursor in one shot. The
// viewport's SetYOffset clamps offset against the current content height.
// Cursor is clamped to a valid link index; anything outside [0, len(Links))
// falls back to -1 (no link selected). Use after SetPage to recover
// scroll/cursor state captured before a navigation.
func (p *PageView) Restore(offset, cursor int) {
	p.vp.SetYOffset(offset)
	if cursor < 0 || cursor >= len(p.result.Links) {
		cursor = -1
	}
	p.cursor = cursor
}

// ScrollToTask centres the viewport on the open todo at the given 0-based
// ordinal (its position among the page's open todos in document order, as
// recorded in render.Result.Tasks). No-op when the ordinal is out of range.
// One-shot deep-link jump applied at SetPage time — not a property Restore
// rewinds into.
func (p *PageView) ScrollToTask(ordinal int) {
	if ordinal < 0 || ordinal >= len(p.result.Tasks) {
		return
	}
	off := p.result.Tasks[ordinal]
	if off < 0 || off > len(p.result.Styled) {
		return
	}
	row := strings.Count(p.result.Styled[:off], "\n")
	target := row - p.vp.Height()/2
	if target < 0 {
		target = 0
	}
	p.vp.SetYOffset(target)
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

func (p *PageView) View() string {
	if p.err != nil {
		return fmt.Sprintf("error: %v", p.err)
	}
	body := p.result.Styled
	if body == "" {
		body = styleFaint.Render("(no entry yet for this page)")
	}
	if p.cursor >= 0 && p.cursor < len(p.result.Links) {
		l := p.result.Links[p.cursor]
		body = body[:l.Start] + cursorStyle.Render(l.Display) + body[l.End:]
	}
	p.vp.SetContent(body)
	return p.vp.View()
}

func (p *PageView) load() {
	p.err = nil
	p.result = render.Result{}
	p.body, p.rows, p.rowsDone = "", nil, false
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
			p.vp.SetContent(p.result.Styled)
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
	p.vp.SetContent(p.result.Styled)
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

// scrollToFirstFind centres the viewport on the first highlighted emphasis
// occurrence, if any.
func (p *PageView) scrollToFirstFind() {
	if len(p.result.Finds) == 0 {
		return
	}
	off := p.result.Finds[0]
	if off < 0 || off > len(p.result.Styled) {
		return
	}
	row := strings.Count(p.result.Styled[:off], "\n")
	target := row - p.vp.Height()/2
	if target < 0 {
		target = 0
	}
	p.vp.SetYOffset(target)
}
