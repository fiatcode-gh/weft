package views

import (
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type Picker struct {
	idx     *graph.Index
	input   textinput.Model
	choices []string // candidate page names (real + virtual journals), sorted by mtime desc
	matches []fuzzy.Match
	sel     int
}

func NewPicker(idx *graph.Index) *Picker {
	ti := textinput.New()
	ti.Placeholder = "Type a page name or YYYY-MM-DD..."
	ti.Focus()
	ti.CharLimit = 200
	p := &Picker{idx: idx, input: ti}
	p.choices = pickerChoices(idx)
	p.search("")
	return p
}

// pickerChoices returns the picker candidate list sorted with the most recently
// active entries first. Real pages use their file mtime; virtual journal dates
// in the ±30-day window get the start of that day so today's journal lands at
// the top alongside any other page edited today.
func pickerChoices(idx *graph.Index) []string {
	type entry struct {
		name  string
		mtime time.Time
	}

	seen := make(map[string]struct{}, len(idx.Pages)+61)
	entries := make([]entry, 0, len(idx.Pages)+61)
	for _, p := range idx.Pages {
		if _, ok := seen[p.Name]; ok {
			continue
		}
		seen[p.Name] = struct{}{}
		entries = append(entries, entry{name: p.Name, mtime: p.ModTime})
	}
	now := time.Now()
	for d := -30; d <= 30; d++ {
		date := now.AddDate(0, 0, d)
		name := date.Format("2006-01-02")
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		entries = append(entries, entry{
			name:  name,
			mtime: time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].mtime.Equal(entries[j].mtime) {
			return entries[i].mtime.After(entries[j].mtime)
		}
		return entries[i].name < entries[j].name
	})
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

func (p *Picker) search(q string) {
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
func (p *Picker) Update(key string) (selected string, accept bool, cancel bool) {
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
	pickerBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	pickerTitle  = lipgloss.NewStyle().Bold(true)
	pickerPrompt = lipgloss.NewStyle().Faint(true)
	pickerSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Bold(true)
	pickerFaint  = lipgloss.NewStyle().Faint(true)
)

// pickerInnerWidth is the column width of the picker body (between the
// border + padding). Picks a comfortable fixed size that fits standard
// terminals; the rounded border auto-expands to fit if a result is wider.
const pickerInnerWidth = 56

func (p *Picker) View() string {
	var b strings.Builder
	b.WriteString(pickerTitle.Render("Find a page"))
	b.WriteString("\n\n")
	b.WriteString(pickerPrompt.Render("> "))
	b.WriteString(p.input.Value())
	b.WriteString("\n")
	b.WriteString(pickerFaint.Render(strings.Repeat("─", pickerInnerWidth)))
	b.WriteString("\n")
	if len(p.matches) == 0 {
		b.WriteString(pickerFaint.Render("  no matches"))
		b.WriteString("\n")
	}
	for i, m := range p.matches {
		marker := "  "
		line := m.Str
		if i == p.sel {
			marker = pickerSel.Render(" ▶ ")
			line = pickerSel.Render(line)
		}
		b.WriteString(marker)
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(pickerFaint.Render("↑/↓ select · enter open · esc cancel"))
	return pickerBorder.Render(b.String())
}
