package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

type Backlinks struct {
	idx    *graph.Index
	target string // page being viewed
	refs   []graph.Ref
	sel    int
	width  int
	height int
}

func NewBacklinks(idx *graph.Index, target string, width, height int) *Backlinks {
	// Self-references (the page mentions its own name) are noise in this
	// view — the user is already on the page. Filter them out before
	// presenting the list.
	src := idx.Backlinks[target]
	refs := make([]graph.Ref, 0, len(src))
	for _, r := range src {
		if r.FromPage == target {
			continue
		}
		refs = append(refs, r)
	}
	return &Backlinks{idx: idx, target: target, refs: refs, width: width, height: height}
}

// SetSize updates the cached terminal dimensions.
func (b *Backlinks) SetSize(w, h int) { b.width, b.height = w, h }

func (b *Backlinks) Update(key string) (selected string, accept, cancel bool) {
	switch key {
	case "esc", "b":
		return "", false, true
	case "up", "k", "ctrl+k":
		if b.sel > 0 {
			b.sel--
		}
	case "down", "j", "ctrl+j":
		if b.sel < len(b.refs)-1 {
			b.sel++
		}
	case "enter":
		if b.sel >= 0 && b.sel < len(b.refs) {
			return b.refs[b.sel].FromPage, true, false
		}
	}
	return "", false, false
}

var (
	blBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	blTitle  = lipgloss.NewStyle().Bold(true)
	blFaint  = lipgloss.NewStyle().Faint(true)
	blPos    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	blSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Bold(true)
)

const (
	blInnerWidthMax  = 80 // matches picker/search/todos for visual uniformity
	blInnerWidthMin  = 30
	blVisibleRowsMax = 14
	blVisibleRowsMin = 6
)

func (b *Backlinks) innerWidth() int {
	w := b.width - 2 - 4 - 4
	if w > blInnerWidthMax {
		w = blInnerWidthMax
	}
	if w < blInnerWidthMin {
		w = blInnerWidthMin
	}
	return w
}

// visibleRows returns how many ref rows the overlay renders at once. Refs
// scroll within this window when there are more of them.
func (b *Backlinks) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) +
	// 1 (divider) + 1 (blank) + 1 (hint) ≈ 9 lines.
	const chrome = 9
	r := b.height - chrome
	if r > blVisibleRowsMax {
		r = blVisibleRowsMax
	}
	if r < blVisibleRowsMin {
		r = blVisibleRowsMin
	}
	return r
}

// scrollWindow returns the [start, end) slice indices of refs to render
// such that b.sel is always visible.
func (b *Backlinks) scrollWindow() (start, end int) {
	rows := b.visibleRows()
	if rows >= len(b.refs) {
		return 0, len(b.refs)
	}
	half := rows / 2
	start = b.sel - half
	if start < 0 {
		start = 0
	}
	end = start + rows
	if end > len(b.refs) {
		end = len(b.refs)
		start = end - rows
	}
	return start, end
}

func (b *Backlinks) View() string {
	inner := b.innerWidth()
	var sb strings.Builder
	sb.WriteString(blTitle.Render("Backlinks"))
	sb.WriteString(blFaint.Render(clamp(fmt.Sprintf("   → %s   (%d)", b.target, len(b.refs)), inner-len("Backlinks"))))
	sb.WriteString("\n\n")
	sb.WriteString(blFaint.Render(strings.Repeat("─", inner)))
	sb.WriteString("\n")
	if len(b.refs) == 0 {
		sb.WriteString(blFaint.Render("  no backlinks"))
		sb.WriteString("\n")
	}
	rowBudget := inner - 3
	start, end := b.scrollWindow()
	if start > 0 {
		sb.WriteString(blFaint.Render(fmt.Sprintf("   ↑ %d more above", start)))
		sb.WriteString("\n")
	}
	for i := start; i < end; i++ {
		r := b.refs[i]
		ctx := strings.TrimSpace(r.Context)
		marker := "   " // 3-cell to match selected " ▶ " width
		var line string
		if i == b.sel {
			marker = blSel.Render(" ▶ ")
			raw := fmt.Sprintf("%s:%d  · %s", r.FromPage, r.LineNumber, ctx)
			line = blSel.Render(clamp(raw, rowBudget))
		} else {
			pos := blPos.Render(fmt.Sprintf("%s:%d", r.FromPage, r.LineNumber))
			line = clamp(pos+blFaint.Render("  · ")+ctx, rowBudget)
		}
		sb.WriteString(marker)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if end < len(b.refs) {
		sb.WriteString(blFaint.Render(fmt.Sprintf("   ↓ %d more below", len(b.refs)-end)))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(blFaint.Render(clamp("↑/↓ select · enter open · b or esc close", inner)))
	return blBorder.Width(inner + 4).Render(sb.String())
}
