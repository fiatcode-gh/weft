package views

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
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
	teatest.RequireEqualOutput(t, []byte(pv.View()))
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
	if row < off || row >= off+pv.vp.Height {
		t.Errorf("target row %d outside viewport [%d,%d)", row, off, off+pv.vp.Height)
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
