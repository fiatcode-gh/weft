package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type Todos struct {
	idx     *graph.Index
	filter  string // "", "TODO", "LATER", "DOING", "WAITING"
	visible []graph.TodoBullet
	sel     int
}

func NewTodos(idx *graph.Index) *Todos {
	t := &Todos{idx: idx}
	t.recompute()
	return t
}

var markerCycle = []string{"", "TODO", "LATER", "DOING", "WAITING"}

func (t *Todos) cycleFilter() {
	for i, m := range markerCycle {
		if m == t.filter {
			t.filter = markerCycle[(i+1)%len(markerCycle)]
			t.recompute()
			return
		}
	}
}

func (t *Todos) recompute() {
	t.visible = t.visible[:0]
	for _, b := range t.idx.Todos {
		if t.filter != "" && b.Marker != t.filter {
			continue
		}
		t.visible = append(t.visible, b)
	}
	sort.SliceStable(t.visible, func(i, j int) bool {
		if t.visible[i].Page != t.visible[j].Page {
			return t.visible[i].Page < t.visible[j].Page
		}
		return t.visible[i].LineNumber < t.visible[j].LineNumber
	})
	if t.sel >= len(t.visible) {
		t.sel = 0
	}
}

func (t *Todos) Update(key string) (page string, accept, cancel bool) {
	switch key {
	case "esc", "q":
		return "", false, true
	case "t":
		t.cycleFilter()
	case "up", "k":
		if t.sel > 0 {
			t.sel--
		}
	case "down", "j":
		if t.sel < len(t.visible)-1 {
			t.sel++
		}
	case "enter":
		if t.sel >= 0 && t.sel < len(t.visible) {
			return t.visible[t.sel].Page, true, false
		}
	}
	return "", false, false
}

var (
	todosBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	todosHdr    = lipgloss.NewStyle().Bold(true)
	todosFaint  = lipgloss.NewStyle().Faint(true)
	todosSel    = lipgloss.NewStyle().Reverse(true)
	todosMark   = map[string]lipgloss.Style{
		"TODO":    lipgloss.NewStyle().Foreground(lipgloss.Color("9")),  // red
		"DOING":   lipgloss.NewStyle().Foreground(lipgloss.Color("11")), // yellow
		"LATER":   lipgloss.NewStyle().Foreground(lipgloss.Color("12")), // blue
		"WAITING": lipgloss.NewStyle().Foreground(lipgloss.Color("8")),  // dim
	}
)

func (t *Todos) View() string {
	var sb strings.Builder
	filterTxt := "all"
	if t.filter != "" {
		filterTxt = t.filter
	}
	sb.WriteString(todosHdr.Render(fmt.Sprintf("Open todos — filter: %s (%d)", filterTxt, len(t.visible))))
	sb.WriteString("\n\n")
	lastPage := ""
	for i, b := range t.visible {
		if b.Page != lastPage {
			if lastPage != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(todosHdr.Render(b.Page))
			sb.WriteString("\n")
			lastPage = b.Page
		}
		marker := b.Marker
		if st, ok := todosMark[b.Marker]; ok {
			marker = st.Render(b.Marker)
		}
		prio := ""
		if b.Priority != "" {
			prio = "[#" + b.Priority + "] "
		}
		row := fmt.Sprintf("  %s %s%s", marker, prio, b.Text)
		if i == t.sel {
			row = todosSel.Render("▶" + row)
		} else {
			row = " " + row
		}
		sb.WriteString(row + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(todosFaint.Render("t cycle filter · enter open · esc back"))
	sb.WriteString("\n")
	return todosBorder.Render(sb.String())
}
