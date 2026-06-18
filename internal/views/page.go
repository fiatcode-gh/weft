package views

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
	"git.fiatcode.dev/fiatcode/weft/v2/internal/render"
)

// renderCount is incremented on every render.Render call inside
// PageView. Tests assert that the per-page cache keeps it from
// growing when re-rendering unchanged pages.
var renderCount int64

// RenderCount returns the current value of the per-page render
// counter. Used by tests.
func RenderCount() int64 { return renderCount }

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
}

type cachedPage struct {
	result  render.Result
	modTime time.Time
}

func NewPageView(idx *graph.Index, page string, width, height int) *PageView {
	pv := &PageView{
		idx:    idx,
		page:   page,
		width:  width,
		height: height,
		cursor: -1,
		cache:  map[string]cachedPage{},
		vp:     viewport.New(width, max(1, height-2)),
	}
	pv.load()
	return pv
}

// Page returns the currently displayed page name.
func (p *PageView) Page() string { return p.page }

// SetPage switches to a different page in the same index.
func (p *PageView) SetPage(name string) {
	p.page = name
	p.cursor = -1
	p.emphasis = ""
	p.load()
}

// SetPageEmphasizing switches to name and highlights whole-word occurrences of
// term (the page navigated from), scrolling to the first. The highlight is
// transient: a later SetPage or history Restore clears it.
func (p *PageView) SetPageEmphasizing(name, term string) {
	p.page = name
	p.cursor = -1
	p.emphasis = term
	p.load()
	p.scrollToFirstFind()
}

// SetSize updates viewport size. The page body is only re-rendered when the
// width changes — height changes don't affect word-wrap, so we just resize
// the viewport and let it re-clip the existing styled content.
func (p *PageView) SetSize(w, h int) {
	widthChanged := w != p.width
	p.width, p.height = w, h
	p.vp.Width = w
	p.vp.Height = max(1, h-2)
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
	if p.cursor < 0 || p.cursor >= len(p.result.Links) {
		return
	}
	l := p.result.Links[p.cursor]
	if l.Start < 0 || l.Start > len(p.result.Styled) {
		return
	}
	row := strings.Count(p.result.Styled[:l.Start], "\n")
	top := p.vp.YOffset
	bottom := top + p.vp.Height - 1
	if row >= top && row <= bottom {
		return
	}
	target := row - p.vp.Height/2
	if target < 0 {
		target = 0
	}
	p.vp.SetYOffset(target)
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
func (p *PageView) LineDown()     { p.vp.LineDown(1) }
func (p *PageView) LineUp()       { p.vp.LineUp(1) }
func (p *PageView) HalfPageDown() { p.vp.HalfViewDown() }
func (p *PageView) HalfPageUp()   { p.vp.HalfViewUp() }
func (p *PageView) GotoTop()      { p.vp.GotoTop() }
func (p *PageView) GotoBottom()   { p.vp.GotoBottom() }

// Offset returns the viewport's current scroll position (YOffset).
func (p *PageView) Offset() int { return p.vp.YOffset }

// Cursor returns the current link cursor index. -1 means no link selected.
func (p *PageView) Cursor() int { return p.cursor }

// ScrollIndicator returns "" when the page fits the viewport (no scroll
// possible), otherwise "NN%" — 0% at the top, 100% at the bottom.
// viewport.ScrollPercent is already clamped to [0, 1] and returns exactly
// 0 at YOffset=0 and 1 at the max offset.
func (p *PageView) ScrollIndicator() string {
	if p.vp.TotalLineCount() <= p.vp.Height {
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
	target := row - p.vp.Height/2
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
	meta, ok := p.idx.Resolve(p.page)
	if !ok {
		return
	}
	// The per-page cache holds only plain (un-emphasised) renders. When an
	// emphasis term is set the render is transient — never read or write the
	// cache, so a later plain navigation can't be served a highlighted version.
	if p.emphasis == "" {
		if c, hit := p.cache[meta.Name]; hit && c.modTime.Equal(meta.ModTime) {
			p.result = c.result
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
	var res render.Result
	if p.emphasis != "" {
		res, err = render.RenderWithEmphasis(body, p.width, p.emphasis)
	} else {
		res, err = render.Render(body, p.width)
	}
	if err != nil {
		p.err = err
		return
	}
	p.result = res
	if p.emphasis == "" {
		p.cache[meta.Name] = cachedPage{result: res, modTime: meta.ModTime}
	}
	p.vp.SetContent(p.result.Styled)
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
	target := row - p.vp.Height/2
	if target < 0 {
		target = 0
	}
	p.vp.SetYOffset(target)
}
