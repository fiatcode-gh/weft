package views

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
	"github.com/fiatcode/logseq-tui/internal/render"
)

// PageView renders a single page with a wiki-link cursor.
type PageView struct {
	idx    *graph.Index
	page   string
	result render.Result
	vp     viewport.Model
	cursor int // index into result.Links, or -1
	err    error
	width  int
	height int
}

func NewPageView(idx *graph.Index, page string, width, height int) *PageView {
	pv := &PageView{
		idx:    idx,
		page:   page,
		width:  width,
		height: height,
		cursor: -1,
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
	p.load()
}

// SetSize updates viewport size.
func (p *PageView) SetSize(w, h int) {
	p.width, p.height = w, h
	p.vp.Width = w
	p.vp.Height = max(1, h-2)
	p.load()
}

// CycleLink moves the cursor to the next/previous wiki-link.
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
}

// FollowCursor returns the link target under the cursor, or "" if none.
func (p *PageView) FollowCursor() string {
	if p.cursor < 0 || p.cursor >= len(p.result.Links) {
		return ""
	}
	return p.result.Links[p.cursor].Target
}

// LineDown/LineUp/HalfPage delegates to viewport.
func (p *PageView) LineDown()     { p.vp.LineDown(1) }
func (p *PageView) LineUp()       { p.vp.LineUp(1) }
func (p *PageView) HalfPageDown() { p.vp.HalfViewDown() }
func (p *PageView) HalfPageUp()   { p.vp.HalfViewUp() }

var cursorStyle = lipgloss.NewStyle().Reverse(true)

func (p *PageView) View() string {
	if p.err != nil {
		return fmt.Sprintf("error: %v", p.err)
	}
	header := fmt.Sprintf("# %s\n", p.page)
	body := p.result.Styled
	if body == "" {
		body = "(no entry yet for this page)"
	}
	if p.cursor >= 0 && p.cursor < len(p.result.Links) {
		l := p.result.Links[p.cursor]
		body = body[:l.Start] + cursorStyle.Render(body[l.Start:l.End]) + body[l.End:]
	}
	p.vp.SetContent(body)
	return header + "\n" + p.vp.View()
}

func (p *PageView) load() {
	meta, ok := p.idx.ByName[p.page]
	if !ok {
		p.result = render.Result{}
		return
	}
	b, err := os.ReadFile(meta.Path)
	if err != nil {
		p.err = err
		return
	}
	res, err := render.Render(strings.TrimSpace(string(b))+"\n", p.width)
	if err != nil {
		p.err = err
		return
	}
	p.result = res
	p.vp.SetContent(p.result.Styled)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
