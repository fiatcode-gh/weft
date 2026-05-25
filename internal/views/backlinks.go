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
	blBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	blSel    = lipgloss.NewStyle().Reverse(true)
)

func (b *Backlinks) View() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Backlinks → %s (%d)\n\n", b.target, len(b.refs))
	if len(b.refs) == 0 {
		sb.WriteString("(none)\n")
		return blBorder.Render(sb.String())
	}
	for i, r := range b.refs {
		line := fmt.Sprintf("%s:%d — %s", r.FromPage, r.LineNumber, strings.TrimSpace(r.Context))
		if i == b.sel {
			line = blSel.Render("▶ " + line)
		} else {
			line = "  " + line
		}
		sb.WriteString(line + "\n")
	}
	return blBorder.Render(sb.String())
}
