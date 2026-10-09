package views

import (
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// foldStore is the read view's per-page state that outlives a PageView: the
// folds a reader set this session and the row map they are drawn with. It is
// never written anywhere. Keyed by PageMeta.Path.
type foldStore map[string]*pageFolds

type pageFolds struct {
	body    string        // the rendered body the entry belongs to
	folded  map[int]bool  // fold lines (Fold.Line) currently folded
	level   int           // last z level: 0 all shown, 1 top-level bullets, 2 headings only
	outline []render.Fold // memo of render.Outline(body); nil until needed
	width   int           // width rows/own were computed for; 0 = none
	rows    []int         // memo of render.SourceRows(body, width, "")
	own     []bool
}

// foldsOf is the page's fold outline, computed once per body.
func (e *pageFolds) foldsOf() []render.Fold {
	if e.outline == nil {
		e.outline = render.Outline(e.body)
		if e.outline == nil {
			e.outline = []render.Fold{}
		}
	}
	return e.outline
}

// entry is the store's record for the loaded page; with create it makes one.
func (p *PageView) entry(create bool) *pageFolds {
	if p.folds == nil || p.path == "" || p.body == "" {
		return nil
	}
	e := p.folds[p.path]
	if e == nil && create {
		e = &pageFolds{body: p.body, folded: map[int]bool{}}
		p.folds[p.path] = e
	}
	return e
}

// dropStaleFolds forgets the folds of a page whose text is no longer the text
// they were set on.
func (p *PageView) dropStaleFolds() {
	if e := p.folds[p.path]; e != nil && e.body != p.body {
		delete(p.folds, p.path)
	}
}

// applyFolds rebuilds which rows are hidden from the folds in the store, then
// keeps top, the cursor row and the link cursor valid for the new rows.
func (p *PageView) applyFolds() {
	p.vis, p.visOf, p.marks = nil, nil, nil
	if e := p.entry(false); e != nil && len(e.folded) > 0 {
		p.hideFolded(e)
	}
	p.top = clampInt(p.top, 0, p.maxTop())
	if p.rowCount() == 0 {
		p.row = -1
		return
	}
	p.row = p.snap(p.row, +1)
	p.follow()
	if p.linkHidden(p.cursor) {
		p.cursor = -1
	}
}

// hideFolded computes vis, visOf and marks for the folded entries of e. With
// no row map nothing is hidden and the folds stay in the entry.
func (p *PageView) hideFolded(e *pageFolds) {
	lines := p.sourceRows()
	if len(lines) == 0 {
		return
	}
	outline := e.foldsOf()
	var hid []bool
	for _, f := range outline {
		if !e.folded[f.Line] {
			continue
		}
		if len(hid) < f.End {
			hid = append(hid, make([]bool, f.End-len(hid))...)
		}
		for l := f.Start; l < f.End; l++ {
			hid[l] = true
		}
	}
	n := min(len(lines), len(p.full))
	vis := make([]int, 0, len(p.full))
	visOf := make([]int, len(p.full))
	last := map[int]int{} // folded line → its last own non-blank row
	for r := range p.full {
		if r < n {
			if l := lines[r]; l >= 0 && l < len(hid) && hid[l] {
				visOf[r] = -1
				continue
			}
		}
		visOf[r] = len(vis)
		vis = append(vis, r)
		if r < n && e.folded[lines[r]] && p.rowOwned(r) && nonBlank(p.full[r]) {
			last[lines[r]] = r
		}
	}
	marks := map[int]int{}
	for _, f := range outline {
		if r, ok := last[f.Line]; ok && e.folded[f.Line] {
			marks[visOf[r]] = f.End - f.Start
		}
	}
	p.vis, p.visOf, p.marks = vis, visOf, marks
}

// refold changes the folds and redraws, keeping the cursor on its row when
// that is still shown, else on the nearest shown row above, and on the same
// screen row.
func (p *PageView) refold(change func()) {
	cf, screen := -1, 0
	if p.row >= 0 {
		cf, screen = p.fullIdx(p.row), p.row-p.top
	}
	change()
	p.applyFolds()
	if cf < 0 || p.rowCount() == 0 {
		return
	}
	for cf > 0 && p.visRow(cf) < 0 {
		cf--
	}
	p.row = p.snap(max(p.visRow(cf), 0), -1)
	p.top = clampInt(p.row-screen, 0, p.maxTop())
	p.follow()
}

const (
	hintNothingToFold = "nothing to fold here"
	hintCannotFold    = "cannot fold this page"
)

// ToggleFold folds or unfolds the heading section or list item the cursor row
// is on, and returns a hint when it cannot.
func (p *PageView) ToggleFold() string {
	if p.body == "" || p.row < 0 {
		return hintNothingToFold
	}
	lines := p.sourceRows()
	if len(lines) == 0 {
		return hintCannotFold
	}
	r := p.fullIdx(p.row)
	if r >= len(lines) || !p.rowOwned(r) {
		return hintNothingToFold
	}
	line, e := lines[r], p.entry(true)
	known := false
	for _, f := range e.foldsOf() {
		if f.Line == line {
			known = true
			break
		}
	}
	if !known {
		return hintNothingToFold
	}
	p.refold(func() {
		if e.folded[line] {
			delete(e.folded, line)
		} else {
			e.folded[line] = true
		}
	})
	return ""
}

// CycleFoldLevel steps the whole page through all shown, top-level bullets
// folded, headings only, replacing any folds set one by one.
func (p *PageView) CycleFoldLevel() string {
	if p.body == "" {
		return hintNothingToFold
	}
	if len(p.sourceRows()) == 0 {
		return hintCannotFold
	}
	e := p.entry(true)
	e.level = (e.level + 1) % 3
	p.refold(func() {
		e.folded = map[int]bool{}
		for _, f := range e.foldsOf() {
			if e.level == 2 && f.Kind == render.FoldHeading || e.level == 1 && f.Kind == render.FoldBullet && f.Level == 0 {
				e.folded[f.Line] = true
			}
		}
	})
	return [...]string{"fold: all shown", "fold: top-level bullets", "fold: headings only"}[e.level]
}

// reveal unfolds every fold whose hidden lines hold body line.
func (p *PageView) reveal(line int) {
	e := p.entry(false)
	if e == nil || len(e.folded) == 0 {
		return
	}
	changed := false
	for _, f := range e.foldsOf() {
		if e.folded[f.Line] && line >= f.Start && line < f.End {
			delete(e.folded, f.Line)
			changed = true
		}
	}
	if changed {
		p.applyFolds()
	}
}

// revealFull unfolds what hides row fr of full, if anything does.
func (p *PageView) revealFull(fr int) {
	if p.visOf != nil && fr < len(p.visOf) && p.visOf[fr] < 0 {
		p.reveal(p.rows[fr])
	}
}

// revealLine unfolds what hides body line, when the line has rows on screen
// to be hidden (lines is the row map).
func (p *PageView) revealLine(line int, lines []int) {
	if p.visOf == nil {
		return
	}
	for r := range min(len(lines), len(p.full)) {
		if lines[r] == line && p.visOf[r] < 0 && nonBlank(p.full[r]) {
			p.reveal(line)
			return
		}
	}
}
