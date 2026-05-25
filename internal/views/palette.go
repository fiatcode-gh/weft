package views

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type Palette struct {
	idx     *graph.Index
	input   textinput.Model
	choices []string // candidate page names (real + virtual journals)
	matches []fuzzy.Match
	sel     int
}

func NewPalette(idx *graph.Index) *Palette {
	ti := textinput.New()
	ti.Placeholder = "Type a page name or YYYY-MM-DD..."
	ti.Focus()
	ti.CharLimit = 200
	p := &Palette{idx: idx, input: ti}
	p.choices = paletteChoices(idx)
	p.search("")
	return p
}

func paletteChoices(idx *graph.Index) []string {
	// Real pages, sorted alphabetically. Listed first so the no-query view
	// shows the actual catalog instead of a wall of date strings (which sort
	// before letter-starting names lexicographically).
	seen := make(map[string]struct{}, len(idx.Pages)+61)
	pages := make([]string, 0, len(idx.Pages))
	for _, p := range idx.Pages {
		if _, ok := seen[p.Name]; ok {
			continue
		}
		seen[p.Name] = struct{}{}
		pages = append(pages, p.Name)
	}
	sort.Strings(pages)

	// Virtual journal dates ±30 days around today, in chronological order
	// (oldest to newest) so the user can scroll forward through time.
	today := time.Now()
	dates := make([]string, 0, 61)
	for d := -30; d <= 30; d++ {
		name := today.AddDate(0, 0, d).Format("2006-01-02")
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		dates = append(dates, name)
	}

	return append(pages, dates...)
}

func (p *Palette) search(q string) {
	if strings.TrimSpace(q) == "" {
		p.matches = nil
		for i, c := range p.choices {
			if i >= 50 {
				break
			}
			p.matches = append(p.matches, fuzzy.Match{Str: c, Index: i})
		}
		p.sel = 0
		return
	}
	p.matches = fuzzy.Find(q, p.choices)
	if len(p.matches) > 50 {
		p.matches = p.matches[:50]
	}
	p.sel = 0
}

// Update handles a key. Returns (selected page name, accept, cancel).
func (p *Palette) Update(key string) (selected string, accept bool, cancel bool) {
	switch key {
	case "esc":
		return "", false, true
	case "enter":
		if p.sel >= 0 && p.sel < len(p.matches) {
			return p.matches[p.sel].Str, true, false
		}
		return "", false, false
	case "up", "ctrl+k":
		if p.sel > 0 {
			p.sel--
		}
		return "", false, false
	case "down", "ctrl+j":
		if p.sel < len(p.matches)-1 {
			p.sel++
		}
		return "", false, false
	}
	// Otherwise feed the key into the text input
	p.input, _ = consumeKey(p.input, key)
	p.search(p.input.Value())
	return "", false, false
}

// consumeKey is a tiny adapter to feed a key string to a textinput.Model.
// Bubble Tea normally sends tea.KeyMsg; we hand-roll just enough for our overlay.
func consumeKey(ti textinput.Model, key string) (textinput.Model, bool) {
	switch key {
	case "backspace":
		v := ti.Value()
		if len(v) > 0 {
			ti.SetValue(v[:len(v)-1])
		}
		return ti, true
	}
	if len(key) == 1 {
		ti.SetValue(ti.Value() + key)
		return ti, true
	}
	return ti, false
}

var (
	paletteBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	paletteSel    = lipgloss.NewStyle().Reverse(true)
)

func (p *Palette) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "> %s\n\n", p.input.Value())
	for i, m := range p.matches {
		line := m.Str
		if i == p.sel {
			line = paletteSel.Render("▶ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	return paletteBorder.Render(b.String())
}
