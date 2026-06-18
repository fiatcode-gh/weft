package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/sahilm/fuzzy"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
)

func TestPickerFiltersOnQuery(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	// Zero out mtimes so the relative-time hint doesn't drift with
	// checkout time. relativeTime returns "" for a zero mtime.
	for i := range p.choices {
		p.choices[i].mtime = time.Time{}
	}
	p.Update("a") // type 'a'
	p.Update("l")
	p.Update("p")
	teatest.RequireEqualOutput(t, []byte(p.View()))
}

func TestRelativeTimeHints(t *testing.T) {
	// Anchor "now" at a fixed mid-month, mid-day point so the day-arithmetic
	// inside relativeTime is unambiguous regardless of when the test runs.
	now := time.Date(2026, time.May, 25, 12, 0, 0, 0, time.UTC)
	d := func(year, month, day int) time.Time {
		return time.Date(year, time.Month(month), day, 12, 0, 0, 0, time.UTC)
	}
	cases := []struct {
		name string
		then time.Time
		want string
	}{
		{"zero value suppressed", time.Time{}, ""},
		{"today", d(2026, 5, 25), "today"},
		{"yesterday", d(2026, 5, 24), "yesterday"},
		{"3 days ago", d(2026, 5, 22), "3 days ago"},
		{"last week", d(2026, 5, 18), "last week"},
		{"2 weeks ago", d(2026, 5, 11), "2 weeks ago"},
		{"3 weeks ago", d(2026, 5, 4), "3 weeks ago"},
		{"2 months ago", d(2026, 3, 25), "2 months ago"},
		{"last year", d(2025, 5, 25), "last year"},
		{"3 years ago", d(2023, 5, 25), "3 years ago"},
		{"tomorrow", d(2026, 5, 26), "tomorrow"},
		{"in 3 days", d(2026, 5, 28), "in 3 days"},
		{"next week", d(2026, 6, 1), "next week"},
		{"in 3 weeks", d(2026, 6, 15), "in 3 weeks"},
		{"in 2 months", d(2026, 7, 25), "in 2 months"},
		{"in N months (far)", d(2026, 9, 25), "in 4 months"},
		{"next year", d(2027, 5, 25), "next year"},
		{"in N years", d(2029, 5, 25), "in 3 years"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := relativeTime(now, c.then); got != c.want {
				t.Errorf("relativeTime(now=%v, then=%v) = %q, want %q",
					now, c.then, got, c.want)
			}
		})
	}
}

func TestConsumeKeyAppendsRune(t *testing.T) {
	ti := textinput.New()
	ti, ok := consumeKey(ti, "x")
	if !ok || ti.Value() != "x" {
		t.Errorf("rune key: want \"x\"/true, got %q/%v", ti.Value(), ok)
	}
}

func TestConsumeKeyBackspace(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("abc")
	ti, ok := consumeKey(ti, "backspace")
	if !ok || ti.Value() != "ab" {
		t.Errorf("backspace: want \"ab\"/true, got %q/%v", ti.Value(), ok)
	}
	ti.SetValue("")
	ti, ok = consumeKey(ti, "backspace")
	if !ok || ti.Value() != "" {
		t.Errorf("backspace on empty: want \"\"/true, got %q/%v", ti.Value(), ok)
	}
}

func TestConsumeKeySpaceVariants(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("a")
	ti, ok := consumeKey(ti, " ")
	if !ok || ti.Value() != "a " {
		t.Errorf("literal space: want \"a \"/true, got %q/%v", ti.Value(), ok)
	}
	ti, ok = consumeKey(ti, "space")
	if !ok || ti.Value() != "a  " {
		t.Errorf("named space: want \"a  \"/true, got %q/%v", ti.Value(), ok)
	}
}

func TestConsumeKeyMultiCharNoop(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("a")
	_, ok := consumeKey(ti, "ctrl+x")
	if ok {
		t.Errorf("multi-char key should not be consumed")
	}
}

func TestPickerUpDownBounds(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	if len(p.matches) < 2 {
		t.Fatalf("setup: need >=2 matches, got %d", len(p.matches))
	}

	p.Update("up")
	if p.sel != 0 {
		t.Errorf("up at top: want sel 0, got %d", p.sel)
	}
	p.Update("down")
	if p.sel != 1 {
		t.Errorf("after down: want sel 1, got %d", p.sel)
	}
	p.Update("ctrl+j")
	if p.sel != 2 && len(p.matches) > 2 {
		t.Errorf("after ctrl+j: want sel 2, got %d", p.sel)
	}
	p.Update("ctrl+k")
	if p.sel < 0 {
		t.Errorf("after ctrl+k: sel went negative, got %d", p.sel)
	}
}

func TestPickerEnterReturnsSelected(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	if len(p.matches) == 0 {
		t.Fatal("setup: no matches")
	}
	want := p.matches[0].Str
	res := p.Update("enter")
	if !res.Accept || res.Cancel {
		t.Errorf("enter: want accept=true cancel=false, got %v/%v", res.Accept, res.Cancel)
	}
	if res.Selected != want {
		t.Errorf("returned name: want %q, got %q", want, res.Selected)
	}
}

// TestPickerCreate_EnterOnIsolatedRow verifies that Enter on a query with no
// fuzzy matches (so the Create row is the only selectable row) returns a
// Create result rather than doing nothing.
func TestPickerCreate_EnterOnIsolatedRow(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	for _, r := range "zzzzzzzzznotapage" {
		p.Update(string(r))
	}
	if len(p.matches) != 0 {
		t.Fatalf("setup: query should yield no matches, got %d", len(p.matches))
	}
	// With the create row present, Enter now returns a Create result.
	res := p.Update("enter")
	if !res.Accept || !res.Create || res.Selected != "zzzzzzzzznotapage" {
		t.Errorf("enter with create row: want Accept+Create+Selected, got (%q,%v,%v,%v)",
			res.Selected, res.Accept, res.Cancel, res.Create)
	}
}

func TestPickerEscCancels(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	res := p.Update("esc")
	if res.Selected != "" || res.Accept || !res.Cancel {
		t.Errorf("esc: want cancel only, got (%q,%v,%v)", res.Selected, res.Accept, res.Cancel)
	}
}

func TestPickerScrollWindowAllFit(t *testing.T) {
	p := &Picker{listBox: listBox{height: 24}}
	p.matches = make([]fuzzy.Match, 5)
	start, end := scrollWindow(p.sel, len(p.matches), p.visibleRows())
	if start != 0 || end != 5 {
		t.Errorf("all-fit: want [0,5), got [%d,%d)", start, end)
	}
}

func TestPickerScrollWindowAtBottom(t *testing.T) {
	p := &Picker{listBox: listBox{height: 16}}
	p.matches = make([]fuzzy.Match, 20)
	p.sel = 19
	start, end := scrollWindow(p.sel, len(p.matches), p.visibleRows())
	if end != 20 || (end-start) != p.visibleRows() {
		t.Errorf("at bottom: want end=20 window=%d, got [%d,%d)",
			p.visibleRows(), start, end)
	}
	if p.sel < start || p.sel >= end {
		t.Errorf("sel %d should be in window [%d,%d)", p.sel, start, end)
	}
}

func TestPickerScrollWindowMiddle(t *testing.T) {
	p := &Picker{listBox: listBox{height: 24}}
	p.matches = make([]fuzzy.Match, 30)
	p.sel = 15
	start, end := scrollWindow(p.sel, len(p.matches), p.visibleRows())
	if p.sel < start || p.sel >= end {
		t.Errorf("sel %d should be in window [%d,%d)", p.sel, start, end)
	}
}

// TestOverlayFootersFitNarrowWidth asserts that each overlay's footer
// hint is clamped so the panel stays bounded at the narrowest terminal
// the inner-width clamp allows (inner = 30). Without clamping the hint,
// the footer line wraps and breaks the panel's rounded border.
func TestOverlayFootersFitNarrowWidth(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)

	cases := []struct {
		name string
		view func() string
	}{
		{"picker", func() string { return NewPicker(idx, 30, 30).View() }},
		{"search", func() string { return NewSearchView(idx, 30, 30).View() }},
		{"backlinks", func() string { return NewBacklinks(idx, "Hub", nil, 30, 30).View() }},
		{"todos", func() string { return NewTodos(idx, 30, 30).View() }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := c.view()
			lines := strings.Split(out, "\n")
			// The first line is the top border; its width sets the panel
			// envelope. Every other line must fit inside the same envelope.
			if len(lines) == 0 {
				t.Fatal("empty view")
			}
			panelW := lipgloss.Width(lines[0])
			for i, l := range lines {
				if w := lipgloss.Width(l); w > panelW {
					t.Errorf("line %d exceeds panel width %d: w=%d, line=%q",
						i, panelW, w, l)
				}
			}
		})
	}
	_ = fuzzy.Match{} // keep fuzzy import
}

func TestPickerSetSize(t *testing.T) {
	p := &Picker{listBox: listBox{width: 80, height: 24}}
	p.SetSize(100, 30)
	if p.width != 100 || p.height != 30 {
		t.Errorf("SetSize: want 100x30, got %dx%d", p.width, p.height)
	}
}

// TestPickerNoSilentCap asserts the picker surfaces every match instead
// of silently truncating at 50. The scroll window handles paging for
// display; capping the underlying slice hid pages a user could otherwise
// scroll to.
func TestPickerNoSilentCap(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	for i := 0; i < 100; i++ {
		meta := graph.PageMeta{
			Name: fmt.Sprintf("synthetic-%03d", i),
			Path: fmt.Sprintf("/synthetic/%03d.md", i),
		}
		idx.Pages = append(idx.Pages, meta)
		idx.ByName[meta.Name] = &idx.Pages[len(idx.Pages)-1]
	}

	p := NewPicker(idx, 80, 30)

	// Empty query: every choice should land in p.matches.
	if got, want := len(p.matches), len(p.choices); got != want {
		t.Errorf("empty-query matches: want %d, got %d", want, got)
	}
	if len(p.matches) < 100 {
		t.Errorf("expected >=100 matches with synthetic pages; got %d", len(p.matches))
	}

	// Typed query: all 100 synthetic pages should match "synthetic".
	for _, r := range "synthetic" {
		p.Update(string(r))
	}
	if got := len(p.matches); got < 100 {
		t.Errorf("typed query should not silently cap; want >=100, got %d", got)
	}
}

func TestConsumeKeyAppendsMultibyteRune(t *testing.T) {
	ti := textinput.New()
	ti, ok := consumeKey(ti, "é")
	if !ok || ti.Value() != "é" {
		t.Errorf("multibyte rune key: want \"é\"/true, got %q/%v", ti.Value(), ok)
	}
	ti, ok = consumeKey(ti, "中")
	if !ok || ti.Value() != "é中" {
		t.Errorf("CJK rune key: want \"é中\"/true, got %q/%v", ti.Value(), ok)
	}
}

func TestConsumeKeyBackspaceRuneAware(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("café")
	ti, ok := consumeKey(ti, "backspace")
	if !ok || ti.Value() != "caf" {
		t.Errorf("rune-aware backspace: want \"caf\"/true, got %q/%v", ti.Value(), ok)
	}

	ti.SetValue("é")
	ti, ok = consumeKey(ti, "backspace")
	if !ok || ti.Value() != "" {
		t.Errorf("backspace emptying a single multibyte rune: want \"\"/true, got %q/%v", ti.Value(), ok)
	}
}

func TestPadTo(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"abc", 5, "abc  "},
		{"abc", 3, "abc"},
		{"abc", 2, "abc"},
		{"", 3, "   "},
	}
	for _, c := range cases {
		if got := padTo(c.in, c.w); got != c.want {
			t.Errorf("padTo(%q,%d): want %q, got %q", c.in, c.w, c.want, got)
		}
	}
}

func typeQuery(p *Picker, s string) {
	for _, r := range s {
		p.Update(string(r))
	}
}

func TestPickerCreate_OfferedForNewName(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "Brand New Page")
	if p.createName != "Brand New Page" {
		t.Errorf("createName: got %q, want %q", p.createName, "Brand New Page")
	}
}

func TestPickerCreate_StripsMdExtension(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "Notes.md")
	if p.createName != "Notes" {
		t.Errorf("createName: got %q, want %q (trailing .md stripped)", p.createName, "Notes")
	}
}

func TestPickerCreate_SuppressedForExistingPage(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "Alpha") // exists in the fixture
	if p.createName != "" {
		t.Errorf("createName should be empty for an existing page; got %q", p.createName)
	}
}

func TestPickerCreate_SuppressedForDateShaped(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "2026-06-15")
	if p.createName != "" {
		t.Errorf("createName should be empty for a date-shaped query; got %q", p.createName)
	}
}

func TestPickerCreate_EnterReturnsCreateResult(t *testing.T) {
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "Zzz New") // no fuzzy matches in fixture
	res := p.Update(keyEnter)
	if !res.Accept || !res.Create || res.Selected != "Zzz New" {
		t.Errorf("enter on create row: got %+v, want {Accept, Create, Selected:\"Zzz New\"}", res)
	}
}

func TestPickerCreate_OfferedWhenNameFuzzyMatchesButDoesNotResolve(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{"pages/Nested Notes Archive.md": "# x\n"})
	p := NewPicker(idx, 80, 30)
	typeQuery(p, "Notes") // fuzzy-matches "Nested Notes Archive" but Resolve("Notes") fails
	if p.createName != "Notes" {
		t.Errorf("createName: got %q, want %q (no page resolves to the exact name)", p.createName, "Notes")
	}
}
