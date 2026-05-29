package views

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
)

func skipIfNoRipgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run search tests")
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
	s.hits = []search.Hit{{FilePath: "x", Line: 1, Context: "y"}}

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
	s.hits = []search.Hit{
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
	s.hits = []search.Hit{
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

	s.Apply(searchDoneMsg{hits: []search.Hit{
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
	s.Apply(searchDoneMsg{hits: []search.Hit{
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

func TestHitLabelKnownVsUnknown(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	s := NewSearchView(idx, 80, 24)

	for _, p := range idx.Pages {
		if got := s.hitLabel(p.Path); got != p.Name {
			t.Errorf("known path %q: want %q, got %q", p.Path, p.Name, got)
		}
		break
	}
	if got, want := s.hitLabel("/totally/elsewhere/file.md"), "elsewhere/file.md"; got != want {
		t.Errorf("unknown path label: want %q, got %q", want, got)
	}
}

func TestMatchesWithin(t *testing.T) {
	s := &SearchView{}
	h := search.Hit{
		Context: "the quick brown fox",
		Matches: []search.Span{{Start: 4, End: 9}, {Start: 10, End: 15}, {Start: 16, End: 19}},
	}
	cases := []struct {
		name  string
		bytes int
		want  []search.Span
	}{
		{"all fit", 100, []search.Span{{Start: 4, End: 9}, {Start: 10, End: 15}, {Start: 16, End: 19}}},
		{"truncates last", 17, []search.Span{{Start: 4, End: 9}, {Start: 10, End: 15}, {Start: 16, End: 17}}},
		{"drops out-of-range", 9, []search.Span{{Start: 4, End: 9}}},
		{"zero budget yields none", 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := s.matchesWithin(h, c.bytes)
			if len(got) != len(c.want) {
				t.Fatalf("len: want %d, got %d (%+v)", len(c.want), len(got), got)
			}
			for i, m := range got {
				if m != c.want[i] {
					t.Errorf("[%d]: want %v, got %v", i, c.want[i], m)
				}
			}
		})
	}
}

func TestMatchesWithinEmpty(t *testing.T) {
	s := &SearchView{}
	if got := s.matchesWithin(search.Hit{Context: "x"}, 10); got != nil {
		t.Errorf("empty Matches: want nil, got %+v", got)
	}
}

func TestHighlightMatches(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")

	if got := highlightMatches("hello world", nil); got != "hello world" {
		t.Errorf("no spans: want verbatim, got %q", got)
	}
	if got := highlightMatches("hi", []search.Span{{Start: 10, End: 12}}); got != "hi" {
		t.Errorf("out-of-range span: want \"hi\", got %q", got)
	}
	if got := highlightMatches("hi", []search.Span{{Start: 1, End: 1}}); got != "hi" {
		t.Errorf("degenerate span: want \"hi\", got %q", got)
	}
	in := "the quick"
	got := highlightMatches(in, []search.Span{{Start: 4, End: 9}, {Start: 0, End: 3}})
	if !strings.Contains(got, "quick") || !strings.HasPrefix(got, "the ") {
		t.Errorf("expected output containing styled \"quick\" with \"the \" prefix; got %q", got)
	}
}

func TestSearchScrollWindow(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.hits = make([]search.Hit, 20)
	for i := range s.hits {
		s.hits[i] = search.Hit{FilePath: "/p/X.md", Line: i + 1, Context: "y"}
	}

	cases := []struct {
		name      string
		sel       int
		wantStart int
	}{
		{"top", 0, 0},
		{"middle", 10, 10 - s.visibleRows()/2},
		{"bottom", 19, len(s.hits) - s.visibleRows()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s.sel = c.sel
			start, end := scrollWindow(s.sel, len(s.hits), s.visibleRows())
			if end-start != s.visibleRows() {
				t.Errorf("window size: want %d, got %d (start=%d end=%d)",
					s.visibleRows(), end-start, start, end)
			}
			if start != c.wantStart {
				t.Errorf("start: want %d, got %d", c.wantStart, start)
			}
			if c.sel < start || c.sel >= end {
				t.Errorf("sel %d should be in [%d,%d)", c.sel, start, end)
			}
		})
	}
}

func TestSearchScrollWindowAllFit(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.hits = []search.Hit{{Line: 1}, {Line: 2}, {Line: 3}}
	start, end := scrollWindow(s.sel, len(s.hits), s.visibleRows())
	if start != 0 || end != 3 {
		t.Errorf("all-fit: want [0,3), got [%d,%d)", start, end)
	}
}

func TestSearchInnerWidthClamps(t *testing.T) {
	s := &SearchView{listBox: listBox{width: 10}}
	if got := s.innerWidth(); got != listInnerWidthMin {
		t.Errorf("narrow term: want min %d, got %d", listInnerWidthMin, got)
	}
	s.width = 200
	if got := s.innerWidth(); got != listInnerWidthMax {
		t.Errorf("wide term: want max %d, got %d", listInnerWidthMax, got)
	}
}

func TestSearchVisibleRowsClamps(t *testing.T) {
	s := &SearchView{listBox: listBox{height: 5}}
	if got := s.visibleRows(); got != listVisibleRowsMin {
		t.Errorf("tiny term: want min %d, got %d", listVisibleRowsMin, got)
	}
	s.height = 100
	if got := s.visibleRows(); got != searchVisibleRowsMax {
		t.Errorf("huge term: want max %d, got %d", searchVisibleRowsMax, got)
	}
}

func TestSearchViewEmptyState(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	teatest.RequireEqualOutput(t, []byte(s.View()))
}

func TestSearchViewWithHits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")
	s.hits = []search.Hit{
		{FilePath: "/abs/pages/Alpha.md", Line: 3, Context: "links to Beta",
			Matches: []search.Span{{Start: 9, End: 13}}},
		{FilePath: "/abs/pages/Hub.md", Line: 2, Context: "the hub mentions Beta in passing",
			Matches: []search.Span{{Start: 17, End: 21}}},
	}
	teatest.RequireEqualOutput(t, []byte(s.View()))
}

func TestShortPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/a/b/c/file.md", "c/file.md"},
		{"a/b.md", "a/b.md"},
		{"singlename", "singlename"},
		{"", ""},
	}
	for _, c := range cases {
		if got := shortPath(c.in); got != c.want {
			t.Errorf("shortPath(%q): want %q, got %q", c.in, c.want, got)
		}
	}
}

func TestSearchCmdRoundtrip(t *testing.T) {
	skipIfNoRipgrep(t)
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}

	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")
	cmd := s.SearchCmd(abs)
	if cmd == nil {
		t.Fatal("SearchCmd returned nil")
	}
	msg := cmd()
	done, ok := msg.(searchDoneMsg)
	if !ok {
		t.Fatalf("want searchDoneMsg, got %T", msg)
	}
	if done.err != nil {
		t.Errorf("done.err: %v", done.err)
	}
	if len(done.hits) == 0 {
		t.Errorf("done.hits: expected at least one")
	}
}
