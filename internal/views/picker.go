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

type pickerChoice struct {
	name  string
	mtime time.Time
}

type Picker struct {
	idx     *graph.Index
	input   textinput.Model
	choices []pickerChoice
	names   []string // derived from choices, kept in lockstep for fuzzy.Find
	matches []fuzzy.Match
	sel     int
	width   int       // terminal width snapshot, for layout
	height  int       // terminal height snapshot, for scroll window sizing
	now     time.Time // captured at construction for stable relative-time hints
}

func NewPicker(idx *graph.Index, width, height int) *Picker {
	ti := textinput.New()
	ti.Placeholder = "Type a page name..."
	ti.Focus()
	ti.CharLimit = 200
	p := &Picker{idx: idx, input: ti, width: width, height: height, now: time.Now()}
	p.choices = pickerChoices(idx)
	p.names = make([]string, len(p.choices))
	for i, c := range p.choices {
		p.names[i] = c.name
	}
	p.search("")
	return p
}

// SetSize updates the cached terminal dimensions.
func (p *Picker) SetSize(w, h int) { p.width, p.height = w, h }

// pickerChoices returns the picker candidate list sorted with the most
// recently modified files first. Only real on-disk pages and journals
// appear; the picker is read-only just like the rest of lstui and won't
// invent rows for files that don't exist yet.
func pickerChoices(idx *graph.Index) []pickerChoice {
	seen := make(map[string]struct{}, len(idx.Pages))
	entries := make([]pickerChoice, 0, len(idx.Pages))
	for _, p := range idx.Pages {
		if _, ok := seen[p.Name]; ok {
			continue
		}
		seen[p.Name] = struct{}{}
		entries = append(entries, pickerChoice{name: p.Name, mtime: p.ModTime})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].mtime.Equal(entries[j].mtime) {
			return entries[i].mtime.After(entries[j].mtime)
		}
		return entries[i].name < entries[j].name
	})
	return entries
}

func (p *Picker) search(q string) {
	if strings.TrimSpace(q) == "" {
		p.matches = nil
		for i, c := range p.choices {
			if i >= 50 {
				break
			}
			p.matches = append(p.matches, fuzzy.Match{Str: c.name, Index: i})
		}
		p.sel = 0
		return
	}
	p.matches = fuzzy.Find(q, p.names)
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
	case "space":
		// Some bubbletea code paths report the space key by name rather
		// than as the literal " " character; handle both forms.
		ti.SetValue(ti.Value() + " ")
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

const (
	pickerInnerWidthMax = 64 // ceiling — picker stays compact even on wide terminals
	pickerInnerWidthMin = 30
	pickerVisibleRowsMax = 14 // ceiling on simultaneously-shown matches
	pickerVisibleRowsMin = 6
)

func (p *Picker) innerWidth() int {
	w := p.width - 2 - 4 - 4 // border + padding + safety margin
	if w > pickerInnerWidthMax {
		w = pickerInnerWidthMax
	}
	if w < pickerInnerWidthMin {
		w = pickerInnerWidthMin
	}
	return w
}

// visibleRows returns how many match rows the picker will render at once.
// The picker scrolls within this window when the match list is longer.
func (p *Picker) visibleRows() int {
	// Picker chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) +
	// 1 (prompt) + 1 (divider) + 1 (blank) + 1 (hint) ≈ 10 lines.
	const chrome = 10
	r := p.height - chrome
	if r > pickerVisibleRowsMax {
		r = pickerVisibleRowsMax
	}
	if r < pickerVisibleRowsMin {
		r = pickerVisibleRowsMin
	}
	return r
}

// scrollWindow returns the [start, end) slice indices of matches to render
// such that p.sel is always visible.
func (p *Picker) scrollWindow() (start, end int) {
	rows := p.visibleRows()
	if rows >= len(p.matches) {
		return 0, len(p.matches)
	}
	half := rows / 2
	start = p.sel - half
	if start < 0 {
		start = 0
	}
	end = start + rows
	if end > len(p.matches) {
		end = len(p.matches)
		start = end - rows
	}
	return start, end
}

func (p *Picker) View() string {
	inner := p.innerWidth()
	const hintCol = 14 // right-aligned mtime hint column width
	nameBudget := inner - hintCol - 2

	var b strings.Builder
	b.WriteString(pickerTitle.Render("Find a page"))
	b.WriteString("\n\n")
	b.WriteString(pickerPrompt.Render("> "))
	b.WriteString(clamp(p.input.Value(), inner-3))
	b.WriteString("\n")
	b.WriteString(pickerFaint.Render(strings.Repeat("─", inner)))
	b.WriteString("\n")
	if len(p.matches) == 0 {
		b.WriteString(pickerFaint.Render("  no matches"))
		b.WriteString("\n")
		b.WriteString("\n")
		b.WriteString(pickerFaint.Render("↑/↓ select · enter open · esc cancel"))
		return pickerBorder.Width(inner + 4).Render(b.String())
	}

	start, end := p.scrollWindow()
	if start > 0 {
		b.WriteString(pickerFaint.Render(fmt.Sprintf("  ↑ %d more above", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		m := p.matches[i]
		hint := ""
		if m.Index >= 0 && m.Index < len(p.choices) {
			hint = relativeTime(p.now, p.choices[m.Index].mtime)
		}
		name := clamp(m.Str, nameBudget)
		marker := "   " // 3-cell marker matches the selected " ▶ " so rows don't shift
		var row string
		if i == p.sel {
			marker = pickerSel.Render(" ▶ ")
			row = pickerSel.Render(layoutPickerRow(name, hint, nameBudget))
		} else if hint != "" {
			row = padTo(name, nameBudget) + "  " + pickerFaint.Render(hint)
		} else {
			row = name
		}
		b.WriteString(marker)
		b.WriteString(row)
		b.WriteString("\n")
	}
	if end < len(p.matches) {
		b.WriteString(pickerFaint.Render(fmt.Sprintf("  ↓ %d more below", len(p.matches)-end)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(pickerFaint.Render("↑/↓ select · enter open · esc cancel"))
	// Width(inner) locks the panel so the rounded border doesn't resize when
	// a longer match scrolls into view.
	return pickerBorder.Width(inner + 4).Render(b.String())
}

// layoutPickerRow returns the plain (unstyled) row layout used for the
// selected-line render. Unselected rows render the same layout but with the
// hint marked faint; doing the styling at the call site keeps the layout
// math simple.
func layoutPickerRow(name, hint string, nameCol int) string {
	if hint == "" {
		return name
	}
	return padTo(name, nameCol) + "  " + hint
}

func padTo(s string, w int) string {
	pad := w - lipgloss.Width(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

// relativeTime returns a coarse human-readable description of t relative to
// now (e.g. "today", "3 days ago", "in 2 weeks"). Returns empty string when
// t is the zero value, which tests use to suppress the hint for stable
// snapshots.
func relativeTime(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	target := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	days := int(today.Sub(target).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days >= 2 && days < 7:
		return fmt.Sprintf("%d days ago", days)
	case days >= 7 && days < 14:
		return "last week"
	case days >= 14 && days < 60:
		return fmt.Sprintf("%d weeks ago", days/7)
	case days >= 60 && days < 365:
		return fmt.Sprintf("%d months ago", days/30)
	case days >= 365:
		years := days / 365
		if years == 1 {
			return "last year"
		}
		return fmt.Sprintf("%d years ago", years)
	case days == -1:
		return "tomorrow"
	case days <= -2 && days > -7:
		return fmt.Sprintf("in %d days", -days)
	case days <= -7 && days > -14:
		return "next week"
	case days <= -14 && days > -60:
		return fmt.Sprintf("in %d weeks", -days/7)
	default:
		return fmt.Sprintf("in %d months", -days/30)
	}
}
