package views

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
)

type pickerChoice struct {
	name  string
	mtime time.Time
}

type Picker struct {
	listBox
	idx        *graph.Index
	input      textinput.Model
	choices    []pickerChoice
	names      []string // derived from choices, kept in lockstep for fuzzy.Find
	matches    []fuzzy.Match
	now        time.Time // captured at construction for stable relative-time hints
	createName string    // non-empty when a "＋ Create" row should be offered
}

func NewPicker(idx *graph.Index, width, height int) *Picker {
	ti := textinput.New()
	ti.Placeholder = "Type a page name..."
	ti.Focus()
	ti.CharLimit = 200
	p := &Picker{listBox: listBox{width: width, height: height}, idx: idx, input: ti, now: time.Now()}
	p.choices = pickerChoices(idx)
	p.names = make([]string, len(p.choices))
	for i, c := range p.choices {
		p.names[i] = c.name
	}
	p.search("")
	return p
}

// pickerChoices returns the picker candidate list sorted with the most
// recently modified files first. Only real on-disk pages and journals
// appear here; the synthetic "＋ Create" row for a not-yet-existent page is
// handled separately (see refreshCreate / createName).
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

// normalizeQuery trims the query and strips a single trailing ".md" so a
// typed extension never doubles to "somepage.md.md".
func normalizeQuery(q string) string {
	q = strings.TrimSpace(q)
	if len(q) >= 3 && strings.EqualFold(q[len(q)-3:], ".md") {
		q = strings.TrimSpace(q[:len(q)-3])
	}
	return q
}

// refreshCreate decides whether to offer a "＋ Create" row for the current
// query: a non-empty, non-date-shaped name that does not resolve to an
// existing page. Fuzzy matches are irrelevant — the user may want a new page
// whose name is a subsequence of an existing one.
func (p *Picker) refreshCreate(q string) {
	name := normalizeQuery(q)
	if name == "" || graph.IsJournalPageName(name) {
		p.createName = ""
		return
	}
	// Gate only on Resolve, not on the fuzzy-match count (see doc comment).
	if _, ok := p.idx.Resolve(name); ok {
		p.createName = ""
		return
	}
	p.createName = name
}

// rowCount is the number of selectable rows: fuzzy matches plus the optional
// create row.
func (p *Picker) rowCount() int {
	if p.createName != "" {
		return len(p.matches) + 1
	}
	return len(p.matches)
}

func (p *Picker) search(q string) {
	if strings.TrimSpace(q) == "" {
		// No query: show all choices (sorted by mtime in pickerChoices).
		// scrollWindow paginates the display; capping the underlying
		// slice would silently hide matches a user could otherwise
		// scroll to.
		p.matches = p.matches[:0]
		for i, c := range p.choices {
			p.matches = append(p.matches, fuzzy.Match{Str: c.name, Index: i})
		}
		p.sel = 0
		p.refreshCreate(q)
		return
	}
	p.matches = fuzzy.Find(q, p.names)
	p.sel = 0
	p.refreshCreate(q)
}

// Update handles a key and reports the result to the App.
func (p *Picker) Update(key string) OverlayResult {
	switch key {
	case keyEsc:
		return OverlayResult{Cancel: true}
	case keyEnter:
		if p.createName != "" && p.sel == len(p.matches) {
			return OverlayResult{Selected: p.createName, Accept: true, Create: true}
		}
		if p.sel >= 0 && p.sel < len(p.matches) {
			return OverlayResult{Selected: p.matches[p.sel].Str, Accept: true}
		}
		return OverlayResult{}
	case keyUp, keyCtrlK:
		p.moveUp()
		return OverlayResult{}
	case keyDown, keyCtrlJ:
		p.moveDown(p.rowCount())
		return OverlayResult{}
	}
	// Otherwise feed the key into the text input.
	p.input, _ = consumeKey(p.input, key)
	p.search(p.input.Value())
	return OverlayResult{}
}

// consumeKey is a tiny adapter to feed a key string to a textinput.Model.
// Bubble Tea normally sends tea.KeyMsg; we hand-roll just enough for our overlay.
func consumeKey(ti textinput.Model, key string) (textinput.Model, bool) {
	switch key {
	case keyBackspace:
		v := ti.Value()
		if len(v) > 0 {
			r := []rune(v)
			ti.SetValue(string(r[:len(r)-1]))
		}
		return ti, true
	case keySpace:
		// Some bubbletea code paths report the space key by name rather
		// than as the literal " " character; handle both forms.
		ti.SetValue(ti.Value() + " ")
		return ti, true
	}
	// A single-rune key string is a printable character to insert. Named keys
	// (enter, esc, up, ctrl+x, …) are always multi-rune, so this also rejects
	// them without an explicit list.
	if utf8.RuneCountInString(key) == 1 {
		ti.SetValue(ti.Value() + key)
		return ti, true
	}
	return ti, false
}

const pickerVisibleRowsMax = 14 // ceiling on simultaneously-shown matches

// visibleRows returns how many match rows the picker will render at once.
// The picker scrolls within this window when the match list is longer.
func (p *Picker) visibleRows() int {
	// Picker chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) +
	// 1 (prompt) + 1 (divider) + 1 (blank) + 1 (hint) ≈ 10 lines.
	const chrome = 10
	return clampInt(p.height-chrome, listVisibleRowsMin, pickerVisibleRowsMax)
}

func (p *Picker) View() string {
	inner := p.innerWidth()
	const hintCol = 14 // right-aligned mtime hint column width
	nameBudget := inner - hintCol - 2

	var b strings.Builder
	b.WriteString(styleTitle.Render("Find a page"))
	b.WriteString("\n\n")
	b.WriteString(styleFaint.Render("> "))
	b.WriteString(clamp(p.input.Value(), inner-3))
	b.WriteString("\n")
	b.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	b.WriteString("\n")
	if len(p.matches) == 0 && p.createName == "" {
		b.WriteString(styleFaint.Render("  no matches"))
		b.WriteString("\n")
		b.WriteString("\n")
		b.WriteString(styleFaint.Render(clamp("↑/↓ select · enter open · esc cancel", inner)))
		return styleBorder.Width(inner + 4).Render(b.String())
	}
	if len(p.matches) == 0 {
		b.WriteString(styleFaint.Render("  no matches"))
		b.WriteString("\n")
	}

	start, end := scrollWindow(p.sel, len(p.matches), p.visibleRows())
	if start > 0 {
		b.WriteString(styleFaint.Render(fmt.Sprintf("  ↑ %d more above", start)))
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
			marker = styleSel.Render(" ▶ ")
			row = styleSel.Render(layoutPickerRow(name, hint, nameBudget))
		} else if hint != "" {
			row = padTo(name, nameBudget) + "  " + styleFaint.Render(hint)
		} else {
			row = name
		}
		b.WriteString(marker)
		b.WriteString(row)
		b.WriteString("\n")
	}
	if end < len(p.matches) {
		b.WriteString(styleFaint.Render(fmt.Sprintf("  ↓ %d more below", len(p.matches)-end)))
		b.WriteString("\n")
	}
	if p.createName != "" {
		marker := "   "
		label := fmt.Sprintf("＋ Create %q", p.createName)
		if p.sel == len(p.matches) {
			marker = styleSel.Render(" ▶ ")
			label = styleSel.Render(label)
		} else {
			label = styleFaint.Render(label)
		}
		b.WriteString(marker)
		b.WriteString(clamp(label, inner))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styleFaint.Render(clamp("↑/↓ select · enter open · esc cancel", inner)))
	// Width(inner) locks the panel so the rounded border doesn't resize when
	// a longer match scrolls into view.
	return styleBorder.Width(inner + 4).Render(b.String())
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
	case days <= -60 && days > -365:
		return fmt.Sprintf("in %d months", -days/30)
	default: // days <= -365
		years := -days / 365
		if years == 1 {
			return "next year"
		}
		return fmt.Sprintf("in %d years", years)
	}
}
