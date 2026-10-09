package views

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// blRow is one rendered row: a non-selectable section header, or a selectable
// reference that is either a linked backlink (ref) or an unlinked one (unl).
type blRow struct {
	header bool
	text   string
	ref    *graph.Ref
	unl    *graph.UnlinkedRef
}

// unlinkedRow reports whether the row is a selectable unlinked reference.
func (r blRow) unlinkedRow() bool { return r.unl != nil }

type Backlinks struct {
	listBox
	idx        *graph.Index
	target     string
	refs       []graph.Ref
	unlinked   []graph.UnlinkedRef
	rows       []blRow
	confirming bool   // l pressed on an unlinked row; preview/confirm is showing
	errMsg     string // failure feedback, rendered inside the panel
}

// NewBacklinks builds the overlay for `target`, combining the in-memory linked
// backlinks with the supplied unlinked references (the App computes those via
// ripgrep on panel-open). Self-references are filtered from the linked list.
func NewBacklinks(idx *graph.Index, target string, unlinked []graph.UnlinkedRef, width, height int) *Backlinks {
	src := idx.BacklinksTo(target)
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
	if b.confirming {
		switch key {
		case keyEnter, "y":
			r := b.rows[b.sel]
			b.confirming = false
			return overlayLinkify(r.unl, b.target)
		case keyEsc, "n":
			b.confirming = false
		}
		return OverlayResult{}
	}
	switch key {
	case keyEsc, "b":
		return overlayCancel()
	case "c":
		return overlayCapture()
	case keyUp, keyK, keyCtrlK:
		b.errMsg = ""
		b.moveSel(-1)
	case keyDown, keyJ, keyCtrlJ:
		b.errMsg = ""
		b.moveSel(+1)
	case "l":
		if b.sel >= 0 && b.sel < len(b.rows) && b.rows[b.sel].unlinkedRow() {
			b.errMsg = ""
			b.confirming = true
		}
	case keyEnter:
		if b.sel >= 0 && b.sel < len(b.rows) {
			r := b.rows[b.sel]
			if r.ref != nil {
				return overlayFocusLink(r.ref.FromPage, b.target)
			}
			if r.unl != nil {
				return overlayHighlight(r.unl.PageName, b.target)
			}
		}
	}
	return OverlayResult{}
}

// SetError records a message to show in the panel and drops the confirm
// sub-state, so the user returns to the list rather than being stuck confirming
// a mention that is gone. The message renders verbatim inside the panel (see
// View) — callers own their own framing (e.g. App.linkify prepends "linkify
// failed: " to its own failures; blockIfSyncing's busy message passes through
// unprefixed). Errors must render inside the overlay — a status-bar hint
// would be invisible behind it.
func (b *Backlinks) SetError(msg string) {
	b.errMsg = msg
	b.confirming = false
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
	switch {
	case b.confirming:
		b.writeConfirm(&sb, inner)
	case b.errMsg != "":
		sb.WriteString(styleTitle.Render(clamp(b.errMsg, inner)))
		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render(clamp(b.hintText(), inner)))
	default:
		sb.WriteString(styleFaint.Render(clamp(b.hintText(), inner)))
	}
	return renderBordered(inner+4, sb.String())
}

// hintText returns the footer key legend, advertising linkify only when the
// selected row is an unlinked reference (the only row l acts on).
func (b *Backlinks) hintText() string {
	if b.sel >= 0 && b.sel < len(b.rows) && b.rows[b.sel].unlinkedRow() {
		return "↑/↓ select · enter open · l linkify · b or esc close"
	}
	return "↑/↓ select · enter open · b or esc close"
}

// writeConfirm renders the preview-and-confirm block for the selected unlinked
// row. The before/after lines are computed purely from the row's Context and
// Match span — no file IO; the authoritative re-match happens at write time.
func (b *Backlinks) writeConfirm(sb *strings.Builder, inner int) {
	u := b.rows[b.sel].unl
	before := u.Context
	// Defensive: a malformed span would panic in View() and crash the TUI. The
	// production pipeline (search.parseJSON) clamps Match into Context, so this
	// falls back to an unchanged preview rather than ever firing in practice.
	after := before
	if u.Match.Start >= 0 && u.Match.Start <= u.Match.End && u.Match.End <= len(u.Context) {
		after = u.Context[:u.Match.Start] + "[[" + u.Context[u.Match.Start:u.Match.End] + "]]" + u.Context[u.Match.End:]
	}
	sb.WriteString(styleFaint.Render(clamp(fmt.Sprintf("── Linkify in %s:%d ", u.PageName, u.Line), inner)))
	sb.WriteString("\n")
	sb.WriteString(clamp("  before:  "+strings.TrimSpace(before), inner))
	sb.WriteString("\n")
	sb.WriteString(clamp("  after:   "+strings.TrimSpace(after), inner))
	sb.WriteString("\n\n")
	sb.WriteString(styleFaint.Render(clamp("  y confirm · n/esc cancel", inner)))
}
