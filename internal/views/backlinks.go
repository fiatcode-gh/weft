package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

// blRow is one rendered row: a non-selectable section header, or a selectable
// reference that is either a linked backlink (ref) or an unlinked one (unl).
type blRow struct {
	header bool
	text   string
	ref    *graph.Ref
	unl    *graph.UnlinkedRef
}

type Backlinks struct {
	listBox
	idx      *graph.Index
	target   string
	refs     []graph.Ref
	unlinked []graph.UnlinkedRef
	rows     []blRow
}

// NewBacklinks builds the overlay for `target`, combining the in-memory linked
// backlinks with the supplied unlinked references (the App computes those via
// ripgrep on panel-open). Self-references are filtered from the linked list.
func NewBacklinks(idx *graph.Index, target string, unlinked []graph.UnlinkedRef, width, height int) *Backlinks {
	src := idx.Backlinks[target]
	refs := make([]graph.Ref, 0, len(src))
	for _, r := range src {
		if r.FromPage == target {
			continue
		}
		refs = append(refs, r)
	}
	b := &Backlinks{
		listBox:  listBox{width: width, height: height},
		idx:      idx,
		target:   target,
		refs:     refs,
		unlinked: unlinked,
	}
	b.rows = b.buildRows()
	b.sel = b.firstSelectable()
	return b
}

// buildRows flattens linked refs, then (when present) an "Unlinked references"
// header followed by the unlinked refs, into one display list.
func (b *Backlinks) buildRows() []blRow {
	rows := make([]blRow, 0, len(b.refs)+len(b.unlinked)+1)
	for i := range b.refs {
		rows = append(rows, blRow{ref: &b.refs[i]})
	}
	if len(b.unlinked) > 0 {
		rows = append(rows, blRow{header: true, text: fmt.Sprintf("Unlinked references  (%d)", len(b.unlinked))})
		for i := range b.unlinked {
			rows = append(rows, blRow{unl: &b.unlinked[i]})
		}
	}
	return rows
}

func (b *Backlinks) firstSelectable() int {
	for i, r := range b.rows {
		if !r.header {
			return i
		}
	}
	return -1
}

// moveSel moves the selection to the next/previous selectable (non-header) row,
// clamped at the ends.
func (b *Backlinks) moveSel(dir int) {
	i := b.sel + dir
	for i >= 0 && i < len(b.rows) {
		if !b.rows[i].header {
			b.sel = i
			return
		}
		i += dir
	}
}

func (b *Backlinks) Update(key string) OverlayResult {
	switch key {
	case keyEsc, "b":
		return OverlayResult{Cancel: true}
	case keyUp, keyK, keyCtrlK:
		b.moveSel(-1)
	case keyDown, keyJ, keyCtrlJ:
		b.moveSel(+1)
	case keyEnter:
		if b.sel >= 0 && b.sel < len(b.rows) {
			r := b.rows[b.sel]
			if r.ref != nil {
				return OverlayResult{Selected: r.ref.FromPage, Accept: true, FocusLinkTo: b.target}
			}
			if r.unl != nil {
				return OverlayResult{Selected: r.unl.PageName, Accept: true, HighlightText: b.target}
			}
		}
	}
	return OverlayResult{}
}

var blPos = lipgloss.NewStyle().Foreground(colorHighlight)

const blVisibleRowsMax = 14

// visibleRows returns how many rows the overlay renders at once. Rows scroll
// within this window when there are more of them.
func (b *Backlinks) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) +
	// 1 (divider) + 1 (blank) + 1 (hint) ≈ 9 lines.
	const chrome = 9
	return clampInt(b.height-chrome, listVisibleRowsMin, blVisibleRowsMax)
}

func (b *Backlinks) View() string {
	inner := b.innerWidth()
	var sb strings.Builder
	sb.WriteString(styleTitle.Render("Backlinks"))
	sb.WriteString(styleFaint.Render(clamp(fmt.Sprintf("   → %s   (%d)", b.target, len(b.refs)), inner-len("Backlinks"))))
	sb.WriteString("\n\n")
	sb.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	sb.WriteString("\n")
	if len(b.rows) == 0 {
		sb.WriteString(styleFaint.Render("  no backlinks"))
		sb.WriteString("\n")
	}
	rowBudget := inner - 3
	start, end := scrollWindow(b.sel, len(b.rows), b.visibleRows())
	if start > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↑ %d more above", start)))
		sb.WriteString("\n")
	}
	for i := start; i < end; i++ {
		r := b.rows[i]
		if r.header {
			sb.WriteString("   ")
			sb.WriteString(styleFaint.Render(clamp("── "+r.text+" ", rowBudget)))
			sb.WriteString("\n")
			continue
		}
		var pos, ctx string
		if r.ref != nil {
			ctx = strings.TrimSpace(r.ref.Context)
			pos = fmt.Sprintf("%s:%d", r.ref.FromPage, r.ref.LineNumber)
		} else {
			ctx = strings.TrimSpace(r.unl.Context)
			pos = fmt.Sprintf("%s:%d", r.unl.PageName, r.unl.Line)
		}
		marker := "   " // 3-cell to match selected " ▶ " width
		var line string
		if i == b.sel {
			marker = styleSel.Render(" ▶ ")
			line = styleSel.Render(clamp(fmt.Sprintf("%s  · %s", pos, ctx), rowBudget))
		} else {
			line = clamp(blPos.Render(pos)+styleFaint.Render("  · ")+ctx, rowBudget)
		}
		sb.WriteString(marker)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if end < len(b.rows) {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↓ %d more below", len(b.rows)-end)))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(styleFaint.Render(clamp("↑/↓ select · enter open · b or esc close", inner)))
	return styleBorder.Width(inner + 4).Render(sb.String())
}
