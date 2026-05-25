package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/peekseq/internal/graph"
)

type Backlinks struct {
	idx    *graph.Index
	target string // page being viewed
	refs   []graph.Ref
	sel    int
	width  int
}

func NewBacklinks(idx *graph.Index, target string, width int) *Backlinks {
	return &Backlinks{idx: idx, target: target, refs: idx.Backlinks[target], width: width}
}

// SetSize updates the cached terminal width.
func (b *Backlinks) SetSize(w, _ int) { b.width = w }

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
	blInnerWidthMax = 80 // matches picker/search/todos for visual uniformity
	blInnerWidthMin = 30
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
	for i, r := range b.refs {
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
	sb.WriteString("\n")
	sb.WriteString(blFaint.Render("↑/↓ select · enter open · b or esc close"))
	return blBorder.Width(inner + 4).Render(sb.String())
}
