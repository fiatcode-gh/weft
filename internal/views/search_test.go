package views

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
	"git.fiatcode.dev/fiatcode/weft/v2/internal/search"
)

func skipIfNoRipgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run search tests")
	}
}

func TestNewSearchViewIndexesPathToName(t *testing.T) {
	quietTerm(t)
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
	quietTerm(t)
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
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	for _, k := range []string{"a", "l", "p"} {
		if res := s.Update(k); res.kind != overlayResultNone {
			t.Errorf("typing %q: want zero result, got %+v", k, res)
		}
	}
	if s.Query() != "alp" {
		t.Errorf("query: want \"alp\", got %q", s.Query())
	}
}

func TestSearchUpdateBackspace(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("abc")
	s.hits = []search.Hit{{FilePath: "x", Line: 1, Context: "y"}}

	s.Update("backspace")
	if s.Query() != "ab" {
		t.Errorf("after first backspace: want \"ab\", got %q", s.Query())
	}
	if s.hits != nil {
		t.Errorf("after backspace: hits must clear, got %+v", s.hits)
	}

	s.Update("backspace")
	s.Update("backspace")
	s.Update("backspace")
	if s.Query() != "" {
		t.Errorf("after draining: want \"\", got %q", s.Query())
	}
}

func TestSearchUpdateSpaceVariants(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("foo")

	s.Update(" ")
	if s.Query() != "foo " {
		t.Errorf("after literal space: want \"foo \", got %q", s.Query())
	}

	s.Update("space")
	if s.Query() != "foo  " {
		t.Errorf("after named space: want \"foo  \", got %q", s.Query())
	}
}

func TestSearchSelectionBounds(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	s.hits = []search.Hit{
		{FilePath: "a", Line: 1, Context: "x"},
		{FilePath: "b", Line: 2, Context: "y"},
		{FilePath: "c", Line: 3, Context: "z"},
	}

	s.Update("up")
	if s.sel != 0 {
		t.Errorf("up at top: want sel 0, got %d", s.sel)
	}
	s.Update("down")
	if s.sel != 1 {
		t.Errorf("after down: want sel 1, got %d", s.sel)
	}
	s.Update("ctrl+j")
	if s.sel != 2 {
		t.Errorf("after ctrl+j: want sel 2, got %d", s.sel)
	}
	s.Update("down")
	if s.sel != 2 {
		t.Errorf("down at bottom: want sel 2, got %d", s.sel)
	}
	s.Update("ctrl+k")
	if s.sel != 1 {
		t.Errorf("after ctrl+k: want sel 1, got %d", s.sel)
	}
}

func TestSearchEnterEmptyQueryNoop(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	if res := s.Update("enter"); res.kind != overlayResultNone {
		t.Errorf("enter on empty query: want zero result, got %+v", res)
	}
}

func TestSearchEnterFirstTimeRunsCmd(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")

	res := s.Update("enter")
	if res.kind != overlayResultCommand || res.cmd == nil {
		t.Errorf("enter to launch: want command result, got %+v", res)
	}
	if !s.running {
		t.Errorf("enter to launch: running flag should be set")
	}
}

func TestSearchEnterWhileRunningIsNoop(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")
	s.running = true

	if res := s.Update("enter"); res.kind != overlayResultNone {
		t.Errorf("enter while running: want zero result, got %+v", res)
	}
}

func TestSearchEnterResolvesHitToPageName(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	s := NewSearchView(idx, 80, 24)
	s.SetQuery("Beta")

	// A hit whose path IS in the index resolves to that page's name.
	want := idx.Pages[0]
	s.hits = []search.Hit{{FilePath: want.Path, Line: 1, Context: "x"}}
	s.sel = 0
	res := s.Update("enter")
	if res.kind != overlayResultOpen || res.page != want.Name {
		t.Errorf("enter with indexed hit: want open %q, got %+v", want.Name, res)
	}

	// A hit whose path is NOT in the index accepts but selects nothing.
	s.hits = []search.Hit{{FilePath: "/totally/unknown/file.md", Line: 1, Context: "x"}}
	s.sel = 0
	res = s.Update("enter")
	if res.kind != overlayResultOpen || res.page != "" {
		t.Errorf("unknown hit should resolve to empty open, got %+v", res)
	}
}

func TestSearchEscCancels(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	res := s.Update("esc")
	if res.kind != overlayResultCancel {
		t.Errorf("esc: want cancel, got %+v", res)
	}
}

func TestSearchApplyPopulatesHits(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	s.running = true
	s.sel = 5

	s.Apply(searchDoneMsg{view: s, gen: s.gen, hits: []search.Hit{
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
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	s.sel = 1
	s.Apply(searchDoneMsg{view: s, gen: s.gen, hits: []search.Hit{
		{FilePath: "/p/A.md", Line: 1}, {FilePath: "/p/B.md", Line: 2},
		{FilePath: "/p/C.md", Line: 3},
	}})
	if s.sel != 1 {
		t.Errorf("sel within new bounds should survive: want 1, got %d", s.sel)
	}
}

func TestSearchApplyRecordsError(t *testing.T) {
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	s.running = true
	s.Apply(searchDoneMsg{view: s, gen: s.gen, err: errors.New("boom")})
	if s.running {
		t.Errorf("after Apply: running should clear even on error")
	}
	if s.err == nil || s.err.Error() != "boom" {
		t.Errorf("error: want \"boom\", got %v", s.err)
	}
}

func TestApplyDropsResultsForEditedQuery(t *testing.T) {
	quietTerm(t)
	// arrange — search "foo" in flight, then the query is edited
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("foo")
	cmd := s.SearchCmd("/nonexistent-graph")
	s.running = true
	s.Update("x") // query is now "foox"; gen bumped

	// act — the stale "foo" result arrives
	msg := cmd().(searchDoneMsg)
	s.Apply(msg)

	// assert
	if s.searched {
		t.Fatal("stale result marked the edited query as searched")
	}
	if s.hits != nil {
		t.Fatalf("stale hits installed: %+v", s.hits)
	}
}

func TestEnterRetriesAfterSearchError(t *testing.T) {
	quietTerm(t)
	// arrange — a failed search must not dead-end the query
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("foo")
	s.running = true
	s.Apply(searchDoneMsg{view: s, gen: s.gen, err: errors.New("rg failed")})

	// act
	res := s.Update(keyEnter)

	// assert — enter re-runs instead of no-op
	if res.kind != overlayResultCommand || res.cmd == nil {
		t.Fatalf("enter after error did not retry the search: %+v", res)
	}
}

func TestHitLabelKnownVsUnknown(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	s := NewSearchView(idx, 80, 24)

	if len(idx.Pages) == 0 {
		t.Fatal("fixture graph has no pages")
	}
	p := idx.Pages[0]
	if got := s.hitLabel(p.Path); got != p.Name {
		t.Errorf("known path %q: want %q, got %q", p.Path, p.Name, got)
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
	quietTerm(t)

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
	quietTerm(t)
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
	quietTerm(t)
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
	quietTerm(t)
	s := NewSearchView(loadFixture(t), 80, 24)
	teatest.RequireEqualOutput(t, []byte(s.View()))
}

func TestSearchViewWithHits(t *testing.T) {
	quietTerm(t)
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

func TestSearchUpdateAcceptsMultibyteRune(t *testing.T) {
	s := NewSearchView(loadFixture(t), 80, 24)
	s.Update("é")
	s.Update("中")
	if s.Query() != "é中" {
		t.Errorf("query after multibyte input: want \"é中\", got %q", s.Query())
	}
	if s.hits != nil {
		t.Errorf("hits should be cleared after multibyte input, got %+v", s.hits)
	}
}

func TestSearchUpdateBackspaceRuneAware(t *testing.T) {
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("café")
	s.Update("backspace")
	if s.Query() != "caf" {
		t.Errorf("rune-aware backspace: want \"caf\", got %q", s.Query())
	}
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
	abs := cloneFixtureGraph(t)
	idx, err := graph.BuildIndex(abs)
	if err != nil {
		t.Fatal(err)
	}

	s := NewSearchView(idx, 80, 24)
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

func TestSearchZeroResultIsNotNeverSearched(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{"pages/Alpha.md": "# Alpha\n"})
	s := NewSearchView(idx, 80, 24)
	s.SetQuery("zzzznomatch")

	s.Apply(searchDoneMsg{view: s, gen: s.gen, hits: nil}) // a completed search with no results
	if !s.searched {
		t.Fatalf("a completed search should set searched=true")
	}
	if got := s.View(); !strings.Contains(got, "no matches") {
		t.Errorf("zero-result view should say 'no matches'; got:\n%s", got)
	}

	// Enter must NOT re-run the same query once we know it returned nothing.
	res := s.Update(keyEnter)
	if res.kind != overlayResultNone {
		t.Errorf("enter after a zero-result search must not re-run the query: %+v", res)
	}
}

func TestSearchEditingQueryResetsSearched(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{"pages/Alpha.md": "# Alpha\n"})
	s := NewSearchView(idx, 80, 24)
	s.SetQuery("zzz")
	s.Apply(searchDoneMsg{view: s, gen: s.gen, hits: nil})
	if !s.searched {
		t.Fatalf("precondition: searched should be true after Apply")
	}

	s.Update("q") // edit the query
	if s.searched {
		t.Errorf("editing the query must reset searched to false")
	}
	if got := s.View(); !strings.Contains(got, "press enter to search") {
		t.Errorf("after editing, view should prompt to search again; got:\n%s", got)
	}
}

func TestSearchHighlightStopsBeforeEllipsis(t *testing.T) {
	// Use colour output so we can inspect SGR codes.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")

	s := NewSearchView(loadFixture(t), 40, 24)
	s.SetQuery("zzz")
	// Context longer than the column budget so clamp appends "…", with a match
	// span that reaches the truncation boundary.
	ctx := strings.Repeat("a", 200) + "zzz"
	s.hits = []search.Hit{{
		FilePath: "/p/X.md",
		Line:     1,
		Context:  ctx,
		Matches:  []search.Span{{Start: len(ctx) - 3, End: len(ctx)}},
	}}
	// Select a non-selected row so highlightMatches is called (sel defaults to 0
	// which selects the first hit; force selection to an out-of-range index).
	s.sel = -1

	view := s.View()
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "…") {
		t.Fatalf("expected an ellipsis in truncated context:\n%s", plain)
	}
	// The match SGR sequence (bold + colour) must not immediately precede the
	// three UTF-8 bytes of "…" (U+2026 = 0xE2 0x80 0xA6). If the ellipsis were
	// coloured by the match style the rendered bytes would contain an SGR open
	// sequence directly followed by those three bytes.
	ellipsisBytes := "\xe2\x80\xa6"
	// An SGR open code always ends in 'm' and is immediately followed by the
	// styled text. Check that wherever "…" appears it is NOT the text right
	// after an SGR opening sequence for the match style.
	if !strings.Contains(view, ellipsisBytes) {
		t.Fatal("ellipsis not found in raw (styled) view")
	}
	// The span is bounded to ctxLimit, so no match SGR may wrap the ellipsis:
	// the match-styled "zzz" must never be immediately followed by the
	// ellipsis bytes.
	matchZzz := searchMatch.Render("zzz")
	if strings.Contains(view, matchZzz+ellipsisBytes) {
		t.Errorf("match SGR wraps the ellipsis: found %q immediately before \"…\" in view", matchZzz)
	}
}
