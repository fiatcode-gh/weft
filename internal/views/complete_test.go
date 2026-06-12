package views

import "testing"

func TestExtractPartial(t *testing.T) {
	cases := []struct {
		name    string
		before  string
		want    string
		wantOK  bool
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
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	c := newLinkCompleter(loadFixture(t))

	c.refresh("just text", "")
	if c.active {
		t.Errorf("should be inactive with no open bracket")
	}

	c.refresh("link to [[Alp", "")
	if !c.active {
		t.Fatalf("should be active for [[Alp")
	}
	cand, ok := c.selected()
	if !ok || cand.name != "Alpha" || cand.create {
		t.Errorf("selected() = (%+v, %v), want Alpha (non-create)", cand, ok)
	}

	c.refresh("[[Zxqv", "")
	got, ok := c.selected()
	if !ok || !got.create || got.name != "Zxqv" {
		t.Errorf("selected() = (%+v, %v), want create row for Zxqv", got, ok)
	}

	c.refresh("[[2026-06-12", "")
	got, ok = c.selected()
	if !ok || !got.create || got.name != "2026-06-12" {
		t.Errorf("date create row: got (%+v, %v), want create row for the date", got, ok)
	}
}

func TestLinkCompleterDismissAndReopen(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	c := newLinkCompleter(loadFixture(t))
	c.refresh("[[Alp", "")
	c.dismiss()
	if c.active {
		t.Errorf("dismiss should deactivate")
	}
	c.refresh("[[Alp", "")
	if c.active {
		t.Errorf("re-deriving the same partial after dismiss should stay inactive")
	}
	c.refresh("[[Alph", "")
	if !c.active {
		t.Errorf("a changed partial should reopen the completer")
	}
}

func TestLinkCompleterEmptyPartialShowsRecent(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	c := newLinkCompleter(loadFixture(t))
	c.refresh("[[", "")
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
	c.refresh("[[Alp", "")
	if !c.active {
		t.Errorf("nil index with a non-empty partial should still offer create")
	}
	c.refresh("[[", "")
	if c.active {
		t.Errorf("nil index with an empty partial has nothing to show")
	}
}

func TestLinkCompleterClosingAhead(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	c := newLinkCompleter(loadFixture(t))
	c.refresh("[[Alp", "]] tail") // cursor inside an already-closed link
	if c.active {
		t.Errorf("must not activate when ]] closes ahead of the cursor")
	}
	c.refresh("[[Alp", " and [[Beta]]") // the ]] ahead belongs to a later link
	if !c.active {
		t.Errorf("should activate; the ]] ahead is part of a separate later link")
	}
}
