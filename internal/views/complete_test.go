package views

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestExtractPartial(t *testing.T) {
	cases := []struct {
		name   string
		before string
		want   string
		wantOK bool
	}{
		{"open bracket simple", "see [[bar", "bar", true},
		{"empty partial just opened", "x [[", "", true},
		{"second open after a closed link", "see [[Foo]] and [[bar", "bar", true},
		{"closed link only", "see [[Foo]] x", "", false},
		{"no bracket at all", "plain text", "", false},
		{"partial contains closing bracket", "[[Foo]", "", false},
		{"page name with spaces stays one partial", "[[Meeting Notes", "Meeting Notes", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractPartial(tc.before)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("extractPartial(%q) = (%q, %v), want (%q, %v)", tc.before, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestLinkCompleterRefresh(t *testing.T) {
	quietTerm(t)
	c := newLinkCompleter(loadFixture(t))

	c.refresh("just text", "", true, true)
	if c.active {
		t.Errorf("should be inactive with no open bracket")
	}

	c.refresh("link to [[Alp", "", true, true)
	if !c.active {
		t.Fatalf("should be active for [[Alp")
	}
	cand, ok := c.selected()
	if !ok || cand.name != "Alpha" || cand.create {
		t.Errorf("selected() = (%+v, %v), want Alpha (non-create)", cand, ok)
	}

	c.refresh("[[Zxqv", "", true, true)
	got, ok := c.selected()
	if !ok || !got.create || got.name != "Zxqv" {
		t.Errorf("selected() = (%+v, %v), want create row for Zxqv", got, ok)
	}

	c.refresh("[[2026-06-12", "", true, true)
	got, ok = c.selected()
	if !ok || !got.create || got.name != "2026-06-12" {
		t.Errorf("date create row: got (%+v, %v), want create row for the date", got, ok)
	}
}

func TestLinkCompleterDismissAndReopen(t *testing.T) {
	quietTerm(t)
	c := newLinkCompleter(loadFixture(t))
	c.refresh("[[Alp", "", true, true)
	c.dismiss()
	if c.active {
		t.Errorf("dismiss should deactivate")
	}
	c.refresh("[[Alp", "", true, true)
	if c.active {
		t.Errorf("re-deriving the same partial after dismiss should stay inactive")
	}
	c.refresh("[[Alph", "", true, true)
	if !c.active {
		t.Errorf("a changed partial should reopen the completer")
	}
}

func TestLinkCompleterEmptyPartialShowsRecent(t *testing.T) {
	quietTerm(t)
	c := newLinkCompleter(loadFixture(t))
	c.refresh("[[", "", true, true)
	if !c.active {
		t.Fatalf("[[ with empty partial should be active")
	}
	if len(c.cands) == 0 {
		t.Errorf("empty partial should list recent pages")
	}
	if c.cands[0].create {
		t.Errorf("recent list should not start with a create row")
	}
}

func TestLinkCompleterNilIndex(t *testing.T) {
	c := newLinkCompleter(nil)
	c.refresh("[[Alp", "", true, true)
	if !c.active {
		t.Errorf("nil index with a non-empty partial should still offer create")
	}
	c.refresh("[[", "", true, true)
	if c.active {
		t.Errorf("nil index with an empty partial has nothing to show")
	}
}

func TestLinkCompleterAllowOpenGate(t *testing.T) {
	quietTerm(t)
	c := newLinkCompleter(loadFixture(t))

	// A bare caret move (allowOpen=false) onto an unclosed [[ must not open a
	// closed strip — completion is a typing affordance.
	c.refresh("[[Alp", "", false, true)
	if c.active {
		t.Errorf("navigation onto [[Alp must not open the strip")
	}

	// An edit (allowOpen=true) opens it...
	c.refresh("[[Alp", "", true, true)
	if !c.active {
		t.Fatalf("an edit producing [[Alp should open the strip")
	}

	// ...and once open, navigation (allowOpen=false) still updates it.
	c.refresh("[[Alph", "", false, true)
	if !c.active || c.partial != "Alph" {
		t.Errorf("an open strip should keep updating on navigation; active=%v partial=%q", c.active, c.partial)
	}

	// Navigation out of the link still closes it.
	c.refresh("no bracket here", "", false, true)
	if c.active {
		t.Errorf("moving the caret out of the link should close the strip")
	}
}

// TestStripHeightMatchesRowsBudget guards the invariant EditorView relies on:
// the strip must render exactly as many rows as rows() reserved, for any
// label length. A label too long for the box's wrap width would spill onto a
// second line, growing the strip past the height EditorView shrank the
// text window by.
func TestStripHeightMatchesRowsBudget(t *testing.T) {
	quietTerm(t)
	cases := []struct {
		name  string
		cands []linkCandidate
	}{
		{
			name:  "long candidate name",
			cands: []linkCandidate{{name: strings.Repeat("x", 34)}},
		},
		{
			// The "＋ Create %q" row adds ~11 cells of decoration around the
			// name, so it must clamp through the same path as a plain row.
			name:  "create row for long partial",
			cands: []linkCandidate{{name: strings.Repeat("y", 34), create: true}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			c := &linkCompleter{active: true, maxVisible: maxCompleterRows, cands: tc.cands}
			// act
			got, want := lipgloss.Height(c.View(40)), c.rows()
			// assert
			if got != want {
				t.Fatalf("strip renders %d rows but rows() reserved %d", got, want)
			}
		})
	}
}

func TestLinkCompleterMoveBoundaries(t *testing.T) {
	// arrange
	c := &linkCompleter{
		active: true,
		sel:    0,
		cands:  []linkCandidate{{name: "One"}, {name: "Two"}, {name: "Three"}},
	}

	// act + assert — up at the top clamps, down walks, up walks back,
	// down at the bottom clamps
	c.moveUp()
	if c.sel != 0 {
		t.Errorf("moveUp at the top must clamp to 0; sel=%d", c.sel)
	}
	c.moveDown()
	if c.sel != 1 {
		t.Errorf("moveDown should advance to 1; sel=%d", c.sel)
	}
	c.moveUp()
	if c.sel != 0 {
		t.Errorf("moveUp should retreat to 0; sel=%d", c.sel)
	}
	c.sel = 2
	c.moveDown()
	if c.sel != 2 {
		t.Errorf("moveDown at the bottom must clamp to 2; sel=%d", c.sel)
	}
}

func TestLinkCompleterClosingAhead(t *testing.T) {
	quietTerm(t)
	c := newLinkCompleter(loadFixture(t))
	c.refresh("[[Alp", "]] tail", true, true) // cursor inside an already-closed link
	if c.active {
		t.Errorf("must not activate when ]] closes ahead of the cursor")
	}
	c.refresh("[[Alp", " and [[Beta]]", true, true) // the ]] ahead belongs to a later link
	if !c.active {
		t.Errorf("should activate; the ]] ahead is part of a separate later link")
	}
}
