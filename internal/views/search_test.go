package views

import (
	"errors"
	"testing"
)

func TestParseRipgrepJSON(t *testing.T) {
	// Two match lines + non-match types interleaved, with one match
	// carrying a real submatch span so we know it's parsed.
	in := `{"type":"begin","data":{"path":{"text":"/g/pages/Alpha.md"}}}
{"type":"match","data":{"path":{"text":"/g/pages/Alpha.md"},"lines":{"text":"links to Beta\n"},"line_number":3,"absolute_offset":42,"submatches":[{"match":{"text":"Beta"},"start":9,"end":13}]}}
{"type":"match","data":{"path":{"text":"/g/journals/2026_05_24.md"},"lines":{"text":"references Alpha\n"},"line_number":1,"absolute_offset":0,"submatches":[]}}
{"type":"end","data":{"path":{"text":"/g/pages/Alpha.md"}}}
`
	hits := parseRipgrepJSON([]byte(in))
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d: %+v", len(hits), hits)
	}
	if hits[0].Line != 3 || hits[0].Context != "links to Beta" {
		t.Errorf("hit[0]: %+v", hits[0])
	}
	if len(hits[0].Matches) != 1 || hits[0].Matches[0].Start != 9 || hits[0].Matches[0].End != 13 {
		t.Errorf("hit[0].Matches: want [{9 13}], got %+v", hits[0].Matches)
	}
	if hits[1].FilePath != "/g/journals/2026_05_24.md" {
		t.Errorf("hit[1]: %+v", hits[1])
	}
	if len(hits[1].Matches) != 0 {
		t.Errorf("hit[1].Matches: want 0, got %d", len(hits[1].Matches))
	}
}

func TestNewSearchViewIndexesPathToName(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	s := NewSearchView(idx, 100, 30)
	if s.width != 100 || s.height != 30 {
		t.Errorf("size: want 100x30, got %dx%d", s.width, s.height)
	}
	// Every indexed page maps its absolute path to its logical name.
	for _, p := range idx.Pages {
		if got := s.pathToName[p.Path]; got != p.Name {
			t.Errorf("pathToName[%q] = %q, want %q", p.Path, got, p.Name)
		}
	}
}

func TestSearchAccessorsAndSetSize(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	if s.Query() != "" {
		t.Errorf("fresh Query: want \"\", got %q", s.Query())
	}
	s.SetQuery("foo")
	if s.Query() != "foo" {
		t.Errorf("after SetQuery: want \"foo\", got %q", s.Query())
	}
	s.SetSize(120, 40)
	if s.width != 120 || s.height != 40 {
		t.Errorf("SetSize: want 120x40, got %dx%d", s.width, s.height)
	}
}

func TestSearchUpdateTypesIntoQuery(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	for _, k := range []string{"a", "l", "p"} {
		hit, accept, cancel, cmd := s.Update(k, "/tmp/x")
		if hit != nil || accept || cancel || cmd != nil {
			t.Errorf("typing %q: want (nil,false,false,nil), got (%v,%v,%v,%v)",
				k, hit, accept, cancel, cmd)
		}
	}
	if s.query != "alp" {
		t.Errorf("query: want \"alp\", got %q", s.query)
	}
}

func TestSearchUpdateBackspace(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("abc")
	s.hits = []SearchHit{{FilePath: "x", Line: 1, Context: "y"}}

	s.Update("backspace", "/tmp/x")
	if s.query != "ab" {
		t.Errorf("after first backspace: want \"ab\", got %q", s.query)
	}
	if s.hits != nil {
		t.Errorf("after backspace: hits must clear, got %+v", s.hits)
	}

	s.Update("backspace", "/tmp/x")
	s.Update("backspace", "/tmp/x")
	s.Update("backspace", "/tmp/x")
	if s.query != "" {
		t.Errorf("after draining: want \"\", got %q", s.query)
	}
}

func TestSearchUpdateSpaceVariants(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("foo")

	s.Update(" ", "/tmp/x")
	if s.query != "foo " {
		t.Errorf("after literal space: want \"foo \", got %q", s.query)
	}

	s.Update("space", "/tmp/x")
	if s.query != "foo  " {
		t.Errorf("after named space: want \"foo  \", got %q", s.query)
	}
}

func TestSearchSelectionBounds(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.hits = []SearchHit{
		{FilePath: "a", Line: 1, Context: "x"},
		{FilePath: "b", Line: 2, Context: "y"},
		{FilePath: "c", Line: 3, Context: "z"},
	}

	s.Update("up", "/tmp/x")
	if s.sel != 0 {
		t.Errorf("up at top: want sel 0, got %d", s.sel)
	}
	s.Update("down", "/tmp/x")
	if s.sel != 1 {
		t.Errorf("after down: want sel 1, got %d", s.sel)
	}
	s.Update("ctrl+j", "/tmp/x")
	if s.sel != 2 {
		t.Errorf("after ctrl+j: want sel 2, got %d", s.sel)
	}
	s.Update("down", "/tmp/x")
	if s.sel != 2 {
		t.Errorf("down at bottom: want sel 2, got %d", s.sel)
	}
	s.Update("ctrl+k", "/tmp/x")
	if s.sel != 1 {
		t.Errorf("after ctrl+k: want sel 1, got %d", s.sel)
	}
}

func TestSearchEnterEmptyQueryNoop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	hit, accept, cancel, cmd := s.Update("enter", "/tmp/x")
	if hit != nil || accept || cancel || cmd != nil {
		t.Errorf("enter on empty query: want all-zero, got (%v,%v,%v,%v)",
			hit, accept, cancel, cmd)
	}
}

func TestSearchEnterFirstTimeRunsCmd(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")

	hit, accept, cancel, cmd := s.Update("enter", "/tmp/x")
	if hit != nil || accept || cancel {
		t.Errorf("enter to launch: want non-accept non-cancel nil hit, got (%v,%v,%v)",
			hit, accept, cancel)
	}
	if cmd == nil {
		t.Errorf("enter to launch: want non-nil cmd, got nil")
	}
	if !s.running {
		t.Errorf("enter to launch: running flag should be set")
	}
}

func TestSearchEnterWhileRunningIsNoop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")
	s.running = true

	hit, accept, cancel, cmd := s.Update("enter", "/tmp/x")
	if hit != nil || accept || cancel || cmd != nil {
		t.Errorf("enter while running: want all-zero, got (%v,%v,%v,%v)",
			hit, accept, cancel, cmd)
	}
}

func TestSearchEnterWithHitsOpensSelection(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")
	s.hits = []SearchHit{
		{FilePath: "/p/Alpha.md", Line: 3, Context: "links to Beta"},
		{FilePath: "/p/Hub.md", Line: 5, Context: "Beta sometimes"},
	}
	s.sel = 1

	hit, accept, cancel, cmd := s.Update("enter", "/tmp/x")
	if !accept || cancel || cmd != nil {
		t.Errorf("enter with hits: want accept, got accept=%v cancel=%v cmd=%v",
			accept, cancel, cmd)
	}
	if hit == nil || hit.FilePath != "/p/Hub.md" || hit.Line != 5 {
		t.Errorf("returned hit: want Hub.md:5, got %+v", hit)
	}
}

func TestSearchEscCancels(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	hit, accept, cancel, cmd := s.Update("esc", "/tmp/x")
	if hit != nil || accept || !cancel || cmd != nil {
		t.Errorf("esc: want cancel only, got (%v,%v,%v,%v)",
			hit, accept, cancel, cmd)
	}
}

func TestSearchApplyPopulatesHits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.running = true
	s.sel = 5

	s.Apply(searchDoneMsg{hits: []SearchHit{
		{FilePath: "/p/A.md", Line: 1, Context: "x"},
		{FilePath: "/p/B.md", Line: 2, Context: "y"},
	}})

	if s.running {
		t.Errorf("after Apply: running should clear, still true")
	}
	if len(s.hits) != 2 {
		t.Errorf("after Apply: hits len want 2, got %d", len(s.hits))
	}
	if s.sel != 0 {
		t.Errorf("after Apply: sel must clamp to 0 when prior sel exceeded new len, got %d", s.sel)
	}
}

func TestSearchApplyKeepsValidSel(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.sel = 1
	s.Apply(searchDoneMsg{hits: []SearchHit{
		{FilePath: "/p/A.md", Line: 1}, {FilePath: "/p/B.md", Line: 2},
		{FilePath: "/p/C.md", Line: 3},
	}})
	if s.sel != 1 {
		t.Errorf("sel within new bounds should survive: want 1, got %d", s.sel)
	}
}

func TestSearchApplyRecordsError(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.running = true
	s.Apply(searchDoneMsg{err: errors.New("boom")})
	if s.running {
		t.Errorf("after Apply: running should clear even on error")
	}
	if s.err == nil || s.err.Error() != "boom" {
		t.Errorf("error: want \"boom\", got %v", s.err)
	}
}
