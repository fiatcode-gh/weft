package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type Backlinks struct {
	idx    *graph.Index
	target string // page being viewed
	refs   []graph.Ref
	sel    int
}

func NewBacklinks(idx *graph.Index, target string) *Backlinks {
	return &Backlinks{idx: idx, target: target, refs: idx.Backlinks[target]}
}

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

const blInnerWidth = 64

func (b *Backlinks) View() string {
	var sb strings.Builder
	sb.WriteString(blTitle.Render("Backlinks"))
	sb.WriteString(blFaint.Render(fmt.Sprintf("   → %s   (%d)", b.target, len(b.refs))))
	sb.WriteString("\n\n")
	sb.WriteString(blFaint.Render(strings.Repeat("─", blInnerWidth)))
	sb.WriteString("\n")
	if len(b.refs) == 0 {
		sb.WriteString(blFaint.Render("  no backlinks"))
		sb.WriteString("\n")
	}
	for i, r := range b.refs {
		ctx := strings.TrimSpace(r.Context)
		marker := "  "
		var line string
		if i == b.sel {
			marker = blSel.Render(" ▶ ")
			line = blSel.Render(fmt.Sprintf("%s:%d  · %s", r.FromPage, r.LineNumber, ctx))
		} else {
			pos := blPos.Render(fmt.Sprintf("%s:%d", r.FromPage, r.LineNumber))
			line = pos + blFaint.Render("  · ") + ctx
		}
		sb.WriteString(marker)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(blFaint.Render("↑/↓ select · enter open · b or esc close"))
	return blBorder.Render(sb.String())
}
