package views

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

func loadFixture(t *testing.T) *graph.Index {
	t.Helper()
	abs := cloneFixtureGraph(t)
	idx, err := graph.BuildIndex(abs)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestPageViewRendersAlpha(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 24)
	teatest.RequireEqualOutput(t, []byte(plain(pv.View())))
}

func TestPageViewRendersActiveLinkCursor(t *testing.T) {
	// Lip Gloss v2 always emits SGR, so the cursor splice lands in the
	// snapshot without forcing a colour profile. Width 77 is unique to this
	// test: the glamour renderer is width-cached process-wide.

	// arrange — the default style, whatever the environment says
	t.Setenv("NO_COLOR", "")
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 77, 24)
	pv.CycleLink(+1)
	if pv.Cursor() != 0 {
		t.Fatalf("precondition: cursor should sit on the first link, got %d", pv.Cursor())
	}

	// act + assert
	teatest.RequireEqualOutput(t, []byte(pv.View()))
}

// Without colour the cursor link must still stand out: reverse video.
func TestPageViewLinkCursorVisibleUnderNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, idx := writeGraph(t, map[string]string{"pages/Solo.md": "- see [[Target]]\n"})
	pv := NewPageView(idx, "Solo", 80, 24)
	pv.CycleLink(+1)
	if pv.Cursor() != 0 {
		t.Fatalf("precondition: cursor should sit on the link, got %d", pv.Cursor())
	}

	rows := frameCells(pv.View(), 80, colorprofile.Ascii)

	for _, c := range cellsShowing(t, rows, "Target") {
		if c.Style.Attrs&uv.AttrReverse == 0 {
			t.Errorf("cursor cell %q is not reverse video: %q", c.Content, c.Style.String())
		}
	}
	assertNoColour(t, rows, "page")
}

// A page whose file disappears mid-session must degrade to an error: line on
// its next reload, not panic on the stale cursor/links. The reload path that
// reaches the disk mid-session is a width change: SetSize with a new width
// skips the width-keyed render cache and re-reads the file.
func TestPageViewVanishedFileRendersError(t *testing.T) {
	// arrange — a page with a link, cursor placed, then the file disappears
	quietTerm(t)
	dir, idx := writeGraph(t, map[string]string{
		"pages/Doomed.md": "- body with a link [[Alpha]]\n",
	})
	pv := NewPageView(idx, "Doomed", 80, 24)
	pv.CycleLink(+1)
	if pv.Cursor() != 0 {
		t.Fatalf("precondition: cursor should sit on the link, got %d", pv.Cursor())
	}
	if err := os.Remove(filepath.Join(dir, "pages", "Doomed.md")); err != nil {
		t.Fatal(err)
	}

	// act — a mid-session reload (terminal width change) re-reads the file
	pv.SetSize(60, 24)

	// assert — no panic, the page reports the failure
	got := plain(pv.View())
	if !strings.Contains(got, "error:") {
		t.Errorf("vanished page should render error:, got:\n%s", got)
	}
}

func TestPageViewOffsetCursorAccessors(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	// Small viewport so HalfPageDown actually moves YOffset against
	// Alpha's ~10 styled lines of content.
	pv := NewPageView(idx, "Alpha", 80, 5)

	if got := pv.Offset(); got != 0 {
		t.Errorf("fresh Offset: want 0, got %d", got)
	}
	if got := pv.Cursor(); got != -1 {
		t.Errorf("fresh Cursor: want -1, got %d", got)
	}

	pv.CycleLink(+1)
	if got := pv.Cursor(); got != 0 {
		t.Errorf("after CycleLink(+1): want cursor 0, got %d", got)
	}

	pv.HalfPageDown()
	if got := pv.Offset(); got == 0 {
		t.Errorf("after HalfPageDown: expected non-zero offset, got 0")
	}
}

func TestPageViewRestoreRoundtrip(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.Restore(2, 1)
	if got := pv.Offset(); got != 2 {
		t.Errorf("Restore offset: want 2, got %d", got)
	}
	if got := pv.Cursor(); got != 1 {
		t.Errorf("Restore cursor: want 1, got %d", got)
	}
}

func TestPageViewRestoreClampsCursor(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 24)

	// Cursor past the link count falls back to -1.
	pv.Restore(0, 9999)
	if got := pv.Cursor(); got != -1 {
		t.Errorf("over-range cursor: want -1, got %d", got)
	}

	// Negative cursor below -1 falls back to -1.
	pv.Restore(0, -5)
	if got := pv.Cursor(); got != -1 {
		t.Errorf("under-range cursor: want -1, got %d", got)
	}
}

func TestPageViewGotoTopBottom(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	// Small viewport so Alpha (~10 styled lines) is scrollable.
	pv := NewPageView(idx, "Alpha", 80, 5)

	// Scroll a bit so GotoTop has somewhere to go back to.
	pv.HalfPageDown()
	if pv.Offset() == 0 {
		t.Fatalf("setup: HalfPageDown should have advanced offset; got 0")
	}

	pv.GotoTop()
	if got := pv.Offset(); got != 0 {
		t.Errorf("after GotoTop: want offset 0, got %d", got)
	}

	pv.GotoBottom()
	if got := pv.Offset(); got == 0 {
		t.Errorf("after GotoBottom: expected non-zero offset, got 0")
	}
}

func TestPageViewScrollIndicatorFitsViewport(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	// Tall viewport so Alpha fits entirely — no scroll possible.
	pv := NewPageView(idx, "Alpha", 80, 100)

	if got := pv.ScrollIndicator(); got != "" {
		t.Errorf("non-scrollable indicator: want \"\", got %q", got)
	}
}

func TestPageViewScrollIndicatorTopBottom(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	// Small viewport so Alpha is scrollable.
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.GotoTop()
	if got := pv.ScrollIndicator(); got != "0%" {
		t.Errorf("at top: want \"0%%\", got %q", got)
	}

	pv.GotoBottom()
	if got := pv.ScrollIndicator(); got != "100%" {
		t.Errorf("at bottom: want \"100%%\", got %q", got)
	}
}

func TestPageViewScrollIndicatorMid(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	// Small viewport against Alpha's ~10 styled lines so a single
	// HalfPageDown from the top lands at a non-boundary scroll position.
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.GotoTop()
	pv.HalfPageDown()

	got := pv.ScrollIndicator()
	matched, err := regexp.MatchString(`^\d{1,2}%$`, got)
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !matched {
		t.Errorf("mid-scroll: want NN%% (1-2 digits), got %q", got)
	}
	// Belt-and-braces: it should be a real mid-scroll value, not a boundary.
	if got == "0%" || got == "100%" {
		t.Errorf("mid-scroll landed on a boundary: %q", got)
	}
}

func TestPageFollowCursorNoLink(t *testing.T) {
	quietTerm(t)
	pv := NewPageView(loadFixture(t), "Alpha", 80, 24)
	if got := pv.FollowCursor(); got != "" {
		t.Errorf("no cursor set: want \"\", got %q", got)
	}
}

func TestPageFollowCursorReturnsTarget(t *testing.T) {
	quietTerm(t)
	pv := NewPageView(loadFixture(t), "Alpha", 80, 24)
	pv.CycleLink(+1)
	if got := pv.FollowCursor(); got == "" {
		t.Errorf("after CycleLink: want a target, got empty")
	}
}

func TestPageViewRenderCacheSkipsRender(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	before := RenderCount()
	a := NewPageView(idx, "Alpha", 80, 24)
	first := RenderCount() - before
	// Force a re-load: a SetPage on the same name should be a no-op
	// for the cache.
	a.SetPage("Alpha")
	after := RenderCount() - before
	if after != first {
		t.Errorf("SetPage on the same page bumped render count: before=%d, after=%d", first, after)
	}
}

func TestPageViewRendersIdenticalBodyOnRevisit(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	a := NewPageView(idx, "Alpha", 80, 24)
	b := NewPageView(idx, "Beta", 80, 24)
	c := NewPageView(idx, "Alpha", 80, 24)
	if a.result.Styled != c.result.Styled {
		t.Errorf("re-rendering Alpha produced different bytes")
	}
	_ = b
}

func TestPageLineUpDownAndHalfPageUp(t *testing.T) {
	quietTerm(t)
	pv := NewPageView(loadFixture(t), "Alpha", 80, 5)

	pv.LineDown()
	afterDown := pv.Offset()
	if afterDown == 0 {
		t.Errorf("after LineDown: expected non-zero offset, got 0")
	}
	pv.LineUp()
	if got := pv.Offset(); got >= afterDown {
		t.Errorf("after LineUp: offset should retreat from %d, got %d", afterDown, got)
	}

	pv.GotoBottom()
	bottom := pv.Offset()
	pv.HalfPageUp()
	if got := pv.Offset(); got >= bottom {
		t.Errorf("after HalfPageUp from bottom: offset should retreat from %d, got %d",
			bottom, got)
	}
}

func TestScrollToTaskCentresOnTodo(t *testing.T) {
	quietTerm(t)
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "- filler bullet number %d\n", i)
	}
	b.WriteString("- TODO deep link target\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "- trailing bullet %d\n", i)
	}
	_, idx := writeGraph(t, map[string]string{"pages/Long.md": b.String()})
	pv := NewPageView(idx, "Long", 80, 20)

	pv.ScrollToTask(0) // the page's single open todo
	off := pv.Offset()
	if off == 0 {
		t.Fatalf("ScrollToTask did not scroll a below-the-fold todo into view")
	}
	// The target row must sit inside the viewport window.
	row := strings.Count(pv.result.Styled[:pv.result.Tasks[0]], "\n")
	if row < off || row >= off+pv.vp.Height() {
		t.Errorf("target row %d outside viewport [%d,%d)", row, off, off+pv.vp.Height())
	}
}

func TestScrollToTaskOutOfRangeNoop(t *testing.T) {
	quietTerm(t)
	pv := NewPageView(loadFixture(t), "Alpha", 80, 24)
	pv.ScrollToTask(99) // no such ordinal
	if pv.Offset() != 0 {
		t.Errorf("out-of-range ScrollToTask should be a no-op, got offset %d", pv.Offset())
	}
}

func TestPageViewFocusLinkTo(t *testing.T) {
	quietTerm(t)
	idx := loadFixture(t)
	pv := NewPageView(idx, "kb/notes", 80, 24) // kb/notes contains [[Hub]]
	pv.FocusLinkTo("Hub")
	if pv.Cursor() < 0 {
		t.Fatalf("FocusLinkTo should place the cursor on the [[Hub]] link")
	}
	if resolved, ok := idx.Resolve(pv.FollowCursor()); !ok || resolved.Name != "Hub" {
		t.Errorf("cursor should be on the Hub link; FollowCursor()=%q", pv.FollowCursor())
	}
	// A page-name the page does not link to → no-op (cursor stays -1).
	pv2 := NewPageView(idx, "kb/notes", 80, 24)
	pv2.FocusLinkTo("Orphan")
	if pv2.Cursor() != -1 {
		t.Errorf("FocusLinkTo to an unlinked page should be a no-op; cursor=%d", pv2.Cursor())
	}
}

func TestSetPageResetsScroll(t *testing.T) {
	// arrange — two pages long enough to scroll at height 10
	quietTerm(t)
	long := strings.Repeat("- line\n", 60)
	_, idx := writeGraph(t, map[string]string{
		"pages/A.md": long,
		"pages/B.md": long,
	})
	p := NewPageView(idx, "A", 80, 10)
	for i := 0; i < 20; i++ {
		p.LineDown()
	}
	if p.Offset() == 0 {
		t.Fatal("precondition: expected page A to be scrolled")
	}

	// act
	p.SetPage("B")

	// assert
	if p.Offset() != 0 {
		t.Fatalf("Offset after SetPage = %d, want 0", p.Offset())
	}
}

func TestResizeRerendersAtNewWidth(t *testing.T) {
	// arrange
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{
		"pages/A.md": "- " + strings.Repeat("word ", 40) + "\n",
	})
	p := NewPageView(idx, "A", 100, 24)
	before := p.result.Styled

	// act
	p.SetSize(40, 24)

	// assert — content must be re-wrapped, not served from the
	// width-100 cache entry
	if p.result.Styled == before {
		t.Fatal("resize served the stale width-100 render")
	}
	for _, line := range strings.Split(p.result.Styled, "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Fatalf("line wider than viewport after resize: %d cells", w)
		}
	}
}

func TestPageViewEmphasizeScrollsThenClears(t *testing.T) {
	quietTerm(t)
	var sb strings.Builder
	for i := 0; i < 80; i++ {
		sb.WriteString("- filler line\n")
	}
	sb.WriteString("- a bare Alpha mention near the bottom\n")
	_, idx := writeGraph(t, map[string]string{"pages/Note.md": sb.String()})
	pv := NewPageView(idx, "Note", 80, 24)
	if len(pv.result.Finds) != 0 {
		t.Fatalf("no emphasis yet → no finds; got %d", len(pv.result.Finds))
	}
	pv.SetPageEmphasizing("Note", "Alpha")
	if len(pv.result.Finds) == 0 {
		t.Fatalf("emphasis should record finds")
	}
	if pv.Offset() == 0 {
		t.Errorf("should scroll toward the bottom mention; offset still 0")
	}
	// Plain navigation clears the highlight and must not serve a cached
	// highlighted render.
	pv.SetPage("Note")
	if pv.emphasis != "" {
		t.Errorf("SetPage should clear emphasis")
	}
	if len(pv.result.Finds) != 0 {
		t.Errorf("plain render must have no finds; got %d", len(pv.result.Finds))
	}
}

// A fallback render must not enter the mtime-keyed cache (a transient
// Glamour failure would stick as an unstyled page until the file changes),
// and its cause must land in the debug log.
func TestPageViewSkipsCacheAndLogsOnRenderFallback(t *testing.T) {
	// arrange
	quietTerm(t)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	orig := renderPage
	t.Cleanup(func() { renderPage = orig })
	renderPage = func(body string, width int, emphasis string) (render.Result, error) {
		return render.Result{Styled: body, FallbackErr: errors.New("boom")}, nil
	}
	_, idx := writeGraph(t, map[string]string{"pages/Alpha.md": "- hi\n"})
	p := NewPageView(idx, "Alpha", 80, 24)
	before := RenderCount()

	// act — a revisit would be served from the cache if the fallback were cached
	p.SetPage("Alpha")

	// assert
	if got := RenderCount() - before; got != 1 {
		t.Fatalf("expected a fresh render on revisit (cache skipped), got %d new renders", got)
	}
	logBytes, err := os.ReadFile(filepath.Join(cache, "weft", "weft.log"))
	if err != nil {
		t.Fatalf("expected fallback logged: %v", err)
	}
	if !strings.Contains(string(logBytes), "render fallback") {
		t.Fatalf("log missing fallback entry:\n%s", logBytes)
	}
}

// anchorLongPage builds a page with `prefix` followed by 60 "line N" bullets.
func anchorLongPage(prefix string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for n := range 60 {
		fmt.Fprintf(&b, "- line %d\n", n)
	}
	return b.String()
}

var anchorLineRe = regexp.MustCompile(`line (\d+)`)

// topRowLine parses N from the "line N" bullet on the page's top visible row.
func topRowLine(t *testing.T, p *PageView) int {
	t.Helper()
	row := strings.Split(p.result.Styled, "\n")[p.Offset()]
	m := anchorLineRe.FindStringSubmatch(row)
	if m == nil {
		t.Fatalf("top row %d is not a line bullet: %q", p.Offset(), row)
	}
	var k int
	fmt.Sscanf(m[1], "%d", &k)
	return k
}

func scrollDown(p *PageView, n int) {
	for range n {
		p.LineDown()
	}
}

func TestPageViewAnchorFollowsScroll(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{
		"pages/A.md": anchorLongPage("- head\n  :LOGBOOK:\n  CLOCK: x\n  :END:\n"),
	})
	p := NewPageView(idx, "A", 80, 10)
	scrollDown(p, 20)
	if p.Offset() <= 0 {
		t.Fatalf("expected scrolled view, offset=%d", p.Offset())
	}
	k := topRowLine(t, p)

	at, ok := p.ReadingAnchor()

	if !ok || at.Line != k+4 || at.RowInLine != 0 || at.ScreenRow != 0 {
		t.Fatalf("ReadingAnchor = (%+v, %v), want line %d on screen row 0", at, ok, k+4)
	}
	if at.Line == p.Offset() {
		t.Fatalf("row and line must diverge; both %d", at.Line)
	}
}

func TestPageViewAnchorAtTopNeedsNoMap(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{
		"pages/A.md": anchorLongPage("- head\n  :LOGBOOK:\n  CLOCK: x\n  :END:\n"),
	})
	p := NewPageView(idx, "A", 80, 10)

	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	sourceRowsFor = func(string, int, string) []int { t.Fatal("row map computed at the top of the page"); return nil }
	// A list-first page shows line 0 on read row 2.
	if at, ok := p.ReadingAnchor(); !ok || at != (Anchor{0, 0, 2}) {
		t.Fatalf("ReadingAnchor = (%+v, %v), want ({0 0 2}, true)", at, ok)
	}
}

func TestPageViewAnchorPrefersVisibleLinkCursor(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{
		"pages/A.md": "- a\n- b\n- c\n- see [[B]]\n- d\n",
		"pages/B.md": "- b\n",
	})
	p := NewPageView(idx, "A", 80, 10)
	p.CycleLink(+1)
	if p.Offset() != 0 || p.Cursor() != 0 {
		t.Fatalf("offset=%d cursor=%d, want 0,0", p.Offset(), p.Cursor())
	}

	at, ok := p.ReadingAnchor()

	if !ok || at != (Anchor{3, 0, 5}) {
		t.Fatalf("ReadingAnchor = (%+v, %v), want ({3 0 5}, true)", at, ok)
	}
}

func TestPageViewAnchorIgnoresOffscreenLinkCursor(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{
		"pages/A.md": anchorLongPage("- see [[B]]\n"),
		"pages/B.md": "- b\n",
	})
	p := NewPageView(idx, "A", 80, 10)
	p.CycleLink(+1)
	scrollDown(p, 20)
	k := topRowLine(t, p)

	at, ok := p.ReadingAnchor()

	if !ok || at.Line != k+1 {
		t.Fatalf("ReadingAnchor = (%+v, %v), want line %d", at, ok, k+1)
	}
}

func TestPageViewAnchorPrefersVisibleLinkCursorWhenScrolled(t *testing.T) {
	quietTerm(t)
	var b strings.Builder
	for n := range 30 {
		fmt.Fprintf(&b, "- line %d\n", n)
	}
	b.WriteString("- see [[B]]\n")
	for n := 31; n < 60; n++ {
		fmt.Fprintf(&b, "- line %d\n", n)
	}
	_, idx := writeGraph(t, map[string]string{"pages/A.md": b.String(), "pages/B.md": "- b\n"})
	p := NewPageView(idx, "A", 80, 10)
	p.CycleLink(+1)
	for {
		r, _ := p.linkRow(p.Cursor())
		if p.Offset() == 0 || r-p.Offset() >= 3 {
			break
		}
		p.LineUp()
	}
	row, found := p.linkRow(p.Cursor())
	if !found || p.Offset() <= 0 || row <= p.Offset() || row > p.Offset()+p.vp.Height()-1 {
		t.Fatalf("setup: link row %d must be visible below top %d", row, p.Offset())
	}
	top := topRowLine(t, p)

	at, ok := p.ReadingAnchor()

	if !ok || at.Line != 30 || at.ScreenRow != row-p.Offset() {
		t.Fatalf("ReadingAnchor = (%+v, %v), want line 30 on screen row %d; top row's line is %d", at, ok, row-p.Offset(), top)
	}
}

func TestPageViewAnchorWithoutRowMapIsNotOK(t *testing.T) {
	quietTerm(t)
	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	sourceRowsFor = func(string, int, string) []int { return nil }
	_, idx := writeGraph(t, map[string]string{"pages/A.md": anchorLongPage("")})
	p := NewPageView(idx, "A", 80, 10)
	scrollDown(p, 20)

	if at, ok := p.ReadingAnchor(); ok || at != (Anchor{}) {
		t.Fatalf("ReadingAnchor = (%+v, %v), want (zero, false)", at, ok)
	}
}

func TestPageViewAnchorComputesMapOncePerLoadAndOnlyWhenNeeded(t *testing.T) {
	quietTerm(t)
	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	calls := 0
	sourceRowsFor = func(body string, width int, emphasis string) []int {
		calls++
		return orig(body, width, emphasis)
	}
	_, idx := writeGraph(t, map[string]string{"pages/A.md": anchorLongPage("")})
	p := NewPageView(idx, "A", 80, 10)

	p.ReadingAnchor() // unscrolled, no cursor: no map needed
	if calls != 0 {
		t.Fatalf("map computed %d times for an unscrolled page, want 0", calls)
	}
	scrollDown(p, 20)
	p.ReadingAnchor()
	p.ReadingAnchor()
	if calls != 1 {
		t.Fatalf("map computed %d times, want 1", calls)
	}
	p.SetPage("A") // cache hit reload must still know its body
	scrollDown(p, 20)
	if at, ok := p.ReadingAnchor(); !ok || at.Line == 0 {
		t.Fatalf("ReadingAnchor after cache-hit reload = (%+v, %v)", at, ok)
	}
	if calls != 2 {
		t.Fatalf("map computed %d times after reload, want 2", calls)
	}
}

// placePage is 60 bullets "- L<n>", the n=20 one wrapped over several rows
// at width 40, with a hidden :LOGBOOK: block (lines 31-33) after L30.
func placePage() string {
	var b strings.Builder
	for n := range 60 {
		switch n {
		case 20:
			b.WriteString("- L20 wrapped alpha beta gamma delta epsilon zeta eta theta iota kappa lambda\n")
		case 31:
			b.WriteString("  :LOGBOOK:\n  CLOCK: x\n  :END:\n")
			fmt.Fprintf(&b, "- L%d\n", n)
		default:
			fmt.Fprintf(&b, "- L%d\n", n)
		}
	}
	return b.String()
}

func TestPlaceAnchor(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{"pages/A.md": placePage()})
	view := func(p *PageView) []string { return strings.Split(plain(p.vp.View()), "\n") }
	rowIndex := func(p *PageView, sub string) int {
		for i, r := range p.styledRows(0, p.vp.TotalLineCount()) {
			if strings.Contains(plain(r), sub) {
				return i
			}
		}
		t.Fatalf("no rendered row holds %q", sub)
		return -1
	}
	tests := []struct {
		name string
		at   Anchor
		// check receives the page after placement.
		check func(t *testing.T, p *PageView)
	}{
		{"wrapped continuation row on screen row 5", Anchor{Line: 20, RowInLine: 1, ScreenRow: 5}, func(t *testing.T, p *PageView) {
			want := plain(p.styledRows(rowIndex(p, "L20")+1, rowIndex(p, "L20")+2)[0])
			if got := view(p)[5]; got != want {
				t.Errorf("screen row 5 = %q, want the second row of the wrapped bullet %q", got, want)
			}
		}},
		{"hidden line falls to the next visible line", Anchor{Line: 32, ScreenRow: 3}, func(t *testing.T, p *PageView) {
			if got := view(p)[3]; !strings.Contains(got, "L31") {
				t.Errorf("screen row 3 = %q, want L31", got)
			}
		}},
		{"past the end lands on the last line", Anchor{Line: 9999, ScreenRow: 3}, func(t *testing.T, p *PageView) {
			rows := view(p)
			if !p.vp.AtBottom() || !strings.Contains(strings.Join(rows, "\n"), "L59") {
				t.Errorf("want the last line L59 at the bottom, offset %d, view:\n%s", p.Offset(), strings.Join(rows, "\n"))
			}
		}},
		{"near the top clamps to offset 0", Anchor{Line: 1, ScreenRow: 6}, func(t *testing.T, p *PageView) {
			if p.Offset() != 0 {
				t.Errorf("offset = %d, want 0", p.Offset())
			}
		}},
		{"near the bottom clamps to the max offset", Anchor{Line: 58, ScreenRow: 0}, func(t *testing.T, p *PageView) {
			if !p.vp.AtBottom() {
				t.Errorf("offset = %d, want the max offset", p.Offset())
			}
		}},
		{"screen row below the read window keeps the line visible", Anchor{Line: 30, ScreenRow: 40}, func(t *testing.T, p *PageView) {
			if got := view(p)[p.vp.Height()-1]; !strings.Contains(got, "L30") {
				t.Errorf("last window row = %q, want L30", got)
			}
		}},
		{"wrapped row past its last row clamps to the last row", Anchor{Line: 20, RowInLine: 99, ScreenRow: 4}, func(t *testing.T, p *PageView) {
			first := rowIndex(p, "L20")
			n := 0
			for _, r := range p.sourceRows()[first:] {
				if r != 20 {
					break
				}
				n++
			}
			if got := p.Offset() + 4; got != first+n-1 {
				t.Errorf("screen row 4 is rendered row %d, want the wrapped bullet's last row %d", got, first+n-1)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPageView(idx, "A", 40, 12)
			p.PlaceAnchor(tt.at)
			if p.Cursor() != -1 {
				t.Errorf("link cursor = %d, want it untouched", p.Cursor())
			}
			tt.check(t, p)
		})
	}
}
