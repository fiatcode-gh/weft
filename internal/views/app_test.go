package views

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// bootApp returns an App that has loaded the fixture index and initialised
// its PageView at a known size. Exposed for history-related tests; mirrors
// the production boot sequence (index load -> WindowSizeMsg -> tryInitPage)
// minus the async hop.
func bootApp(t *testing.T) *App {
	t.Helper()
	// Match page_test.go's environment so the package-wide lipgloss color
	// profile is not primed with truecolor by whichever test runs first.
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	a := New(abs, "test")
	// Drive the deferred index build synchronously.
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	a.Update(msg)
	// Provide a real terminal size so tryInitPage can construct PageView.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	return a
}

func TestHistorySeededOnFirstPage(t *testing.T) {
	a := bootApp(t)
	if len(a.hist) != 1 {
		t.Fatalf("hist len: want 1, got %d", len(a.hist))
	}
	if a.histIdx != 0 {
		t.Errorf("histIdx: want 0, got %d", a.histIdx)
	}
	if a.hist[0].page != a.page.Page() {
		t.Errorf("seed entry page: want %q, got %q", a.page.Page(), a.hist[0].page)
	}
	if a.hist[0].offset != 0 || a.hist[0].cursor != -1 {
		t.Errorf("seed offset/cursor: want 0/-1, got %d/%d",
			a.hist[0].offset, a.hist[0].cursor)
	}
}

func TestNavigatePushesAndCapturesDeparting(t *testing.T) {
	a := bootApp(t)

	// The bootApp page is today's journal, which typically has no file in
	// the fixture (so scroll/cursor mutations are no-ops). Jump to Alpha
	// first to get a page with real content + links to mutate.
	a.navigate("Alpha")
	if a.page.Page() != "Alpha" {
		t.Fatalf("setup: want Alpha, got %q", a.page.Page())
	}

	// Move cursor on Alpha so we have something non-trivial to capture.
	a.page.CycleLink(+1)
	a.page.CycleLink(+1)
	wantOffset := a.page.Offset()
	wantCursor := a.page.Cursor()
	if wantCursor < 0 {
		t.Fatalf("setup: expected cursor to advance on Alpha, got %d", wantCursor)
	}

	a.navigate("Beta")

	if a.page.Page() != "Beta" {
		t.Errorf("after navigate: page want Beta, got %q", a.page.Page())
	}
	if len(a.hist) != 3 {
		t.Fatalf("hist len: want 3, got %d", len(a.hist))
	}
	if a.histIdx != 2 {
		t.Errorf("histIdx: want 2, got %d", a.histIdx)
	}
	if a.hist[1].page != "Alpha" {
		t.Errorf("entry[1].page: want Alpha, got %q", a.hist[1].page)
	}
	if a.hist[1].offset != wantOffset || a.hist[1].cursor != wantCursor {
		t.Errorf("entry[1] captured state: want %d/%d, got %d/%d",
			wantOffset, wantCursor, a.hist[1].offset, a.hist[1].cursor)
	}
	if a.hist[2].page != "Beta" || a.hist[2].offset != 0 || a.hist[2].cursor != -1 {
		t.Errorf("entry[2]: want {Beta 0 -1}, got %+v", a.hist[2])
	}
}

func TestNavigateTruncatesForwardHistory(t *testing.T) {
	a := bootApp(t)

	// Build hist: [journal, Alpha, Beta], histIdx=2.
	a.navigate("Alpha")
	a.navigate("Beta")
	if len(a.hist) != 3 {
		t.Fatalf("setup: want hist len 3, got %d", len(a.hist))
	}

	// Simulate the user being mid-history (a future back-key step).
	// histIdx=1 points at Alpha with Beta still in the forward slot.
	a.histIdx = 1

	// Navigate to a new page from mid-history. Beta must be discarded.
	a.navigate("proj/nested")

	if len(a.hist) != 3 {
		t.Errorf("after branch: hist len want 3, got %d", len(a.hist))
	}
	if a.histIdx != 2 {
		t.Errorf("histIdx: want 2, got %d", a.histIdx)
	}
	if a.hist[2].page != "proj/nested" {
		t.Errorf("tail entry: want proj/nested, got %q", a.hist[2].page)
	}
	// Beta must be gone from the stack.
	for i, e := range a.hist {
		if e.page == "Beta" {
			t.Errorf("Beta still in history at idx %d", i)
		}
	}
}

func TestHistoryBackForward(t *testing.T) {
	a := bootApp(t)
	startPage := a.page.Page()

	a.navigate("Alpha")
	a.navigate("Beta")

	// Press '['
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if got := a.page.Page(); got != "Alpha" {
		t.Errorf("after [: want Alpha, got %q", got)
	}

	// '[' again -> startPage
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if got := a.page.Page(); got != startPage {
		t.Errorf("after second [: want %q, got %q", startPage, got)
	}

	// ']' -> Alpha
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if got := a.page.Page(); got != "Alpha" {
		t.Errorf("after ]: want Alpha, got %q", got)
	}
}

func TestHistoryBackAtStartNoop(t *testing.T) {
	a := bootApp(t)
	startPage := a.page.Page()
	startIdx := a.histIdx

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})

	if got := a.page.Page(); got != startPage {
		t.Errorf("page should be unchanged: want %q, got %q", startPage, got)
	}
	if a.histIdx != startIdx {
		t.Errorf("histIdx should be unchanged: want %d, got %d", startIdx, a.histIdx)
	}
}

func TestHistoryForwardAtTailNoop(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	tailIdx := a.histIdx

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})

	if got := a.page.Page(); got != "Alpha" {
		t.Errorf("page should be unchanged: want Alpha, got %q", got)
	}
	if a.histIdx != tailIdx {
		t.Errorf("histIdx should be unchanged: want %d, got %d", tailIdx, a.histIdx)
	}
}

func TestHistoryBranchTruncatesForward(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	a.navigate("Beta")

	// Back to Alpha; forward stack still has Beta.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if a.page.Page() != "Alpha" {
		t.Fatalf("setup: want Alpha, got %q", a.page.Page())
	}

	// Branch: navigate to a different page. Forward (Beta) must be dropped.
	a.navigate("proj/nested")

	if a.page.Page() != "proj/nested" {
		t.Errorf("after branch: want proj/nested, got %q", a.page.Page())
	}
	if a.histIdx != len(a.hist)-1 {
		t.Errorf("after branch: histIdx should be at tail, got %d (len %d)",
			a.histIdx, len(a.hist))
	}

	// ']' is now a no-op — there is no forward history.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if a.page.Page() != "proj/nested" {
		t.Errorf("forward after branch should be no-op: got %q", a.page.Page())
	}
}

func TestHistoryRestoresScrollAndCursor(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")

	// On Alpha: scroll + select a link.
	a.page.CycleLink(+1)
	a.page.CycleLink(+1)
	wantCursor := a.page.Cursor()

	a.navigate("Beta")

	// Back to Alpha — cursor should be restored.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if a.page.Page() != "Alpha" {
		t.Fatalf("after [: want Alpha, got %q", a.page.Page())
	}
	if got := a.page.Cursor(); got != wantCursor {
		t.Errorf("restored cursor: want %d, got %d", wantCursor, got)
	}
}

func TestPageEdgeKeys(t *testing.T) {
	a := bootApp(t)
	// Navigate to Alpha so we have real content that can scroll. Boot
	// page (today's journal) is typically empty in the fixture.
	a.navigate("Alpha")

	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	// 'G' jumps to bottom.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	bottomOffset := a.page.Offset()
	if bottomOffset == 0 {
		t.Errorf("after G: expected non-zero offset, got 0")
	}

	// 'g' jumps back to top.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if got := a.page.Offset(); got != 0 {
		t.Errorf("after g: want offset 0, got %d", got)
	}
}

func TestAppStatusBarContainsScrollIndicator(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	a.page.GotoBottom()

	bar := a.statusBar()
	if !strings.Contains(bar, "100%") {
		t.Errorf("status bar missing 100%% indicator; got:\n%s", bar)
	}
	if !strings.Contains(bar, "? help") {
		t.Errorf("status bar missing close hint; got:\n%s", bar)
	}
}

func TestAppStatusBarHidesIndicatorWhenFits(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	// Force a very tall viewport so Alpha is guaranteed to fit regardless
	// of future content changes.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 200})

	bar := a.statusBar()
	if strings.Contains(bar, "%") {
		t.Errorf("status bar should hide percentage when page fits; got:\n%s", bar)
	}
	if !strings.Contains(bar, "? help") {
		t.Errorf("status bar missing close hint; got:\n%s", bar)
	}
}

func TestAppOpensAndCancelsPicker(t *testing.T) {
	a := bootApp(t)
	if a.mode != modePage {
		t.Fatalf("setup: want modePage, got %d", a.mode)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if a.mode != modePicker || a.picker == nil {
		t.Fatalf("after ctrl+p: want modePicker with picker set, got mode=%d picker=%v", a.mode, a.picker)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.mode != modePage || a.picker != nil {
		t.Errorf("after esc: want modePage with picker cleared, got mode=%d picker=%v", a.mode, a.picker)
	}
}

func TestAppOpensSearchAndEscapes(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if a.mode != modeSearch || a.search == nil {
		t.Fatalf("after /: want modeSearch with search set, got mode=%d search=%v", a.mode, a.search)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.mode != modePage || a.search != nil {
		t.Errorf("after esc: want modePage with search cleared, got mode=%d search=%v", a.mode, a.search)
	}
}

func TestAppOpensBacklinksAndCloses(t *testing.T) {
	a := bootApp(t)
	a.navigate("Hub")
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if a.mode != modeBacklinks || a.backlinks == nil {
		t.Fatalf("after b: want modeBacklinks, got mode=%d backlinks=%v", a.mode, a.backlinks)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if a.mode != modePage || a.backlinks != nil {
		t.Errorf("after second b: want modePage cleared, got mode=%d backlinks=%v", a.mode, a.backlinks)
	}
}

func TestAppOpensTodosAndCloses(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if a.mode != modeTodos || a.todos == nil {
		t.Fatalf("after T: want modeTodos, got mode=%d todos=%v", a.mode, a.todos)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.mode != modePage || a.todos != nil {
		t.Errorf("after esc: want modePage cleared, got mode=%d todos=%v", a.mode, a.todos)
	}
}

func TestAppOpensHelpAndCloses(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if a.mode != modeHelp || a.help == nil {
		t.Fatalf("after ?: want modeHelp, got mode=%d help=%v", a.mode, a.help)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.mode != modePage || a.help != nil {
		t.Errorf("after esc: want modePage cleared, got mode=%d help=%v", a.mode, a.help)
	}
}

func TestAppPickerAcceptNavigates(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if a.mode != modePicker {
		t.Fatalf("setup: picker not open")
	}
	for _, r := range "Alp" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.mode != modePage {
		t.Errorf("after enter: want modePage, got %d", a.mode)
	}
	if a.page.Page() != "Alpha" {
		t.Errorf("after picker accept: want page Alpha, got %q", a.page.Page())
	}
}

func TestAppTodosAcceptNavigates(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if a.mode != modeTodos {
		t.Fatalf("setup: todos not open")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.mode != modePage {
		t.Errorf("after enter: want modePage, got %d", a.mode)
	}
	if a.todos != nil {
		t.Errorf("todos not cleared")
	}
}

func TestAppLoadingSplashBeforePage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/nonexistent/before/build", "test")
	v := a.View()
	if !strings.Contains(v, "peekseq") {
		t.Errorf("splash should contain title; got:\n%s", v)
	}
	if !strings.Contains(v, "Loading") {
		t.Errorf("splash should announce loading; got:\n%s", v)
	}
}

func TestAppErrorSplashOnIndexFailure(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/some/graph", "test")
	a.Update(indexLoadedMsg{err: errors.New("synthetic build failure")})
	v := a.View()
	if !strings.Contains(v, "failed to index") {
		t.Errorf("error splash should announce failure; got:\n%s", v)
	}
	if !strings.Contains(v, "R to retry") {
		t.Errorf("error splash should show retry hint; got:\n%s", v)
	}
}

func TestAppRetryFromErrorSplash(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/some/graph", "test")
	a.Update(indexLoadedMsg{err: errors.New("synthetic build failure")})
	if a.loadErr == nil {
		t.Fatalf("setup: loadErr should be set")
	}

	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil {
		t.Fatalf("R from error splash should return a retry cmd")
	}
	if a.loadErr != nil {
		t.Errorf("R should clear loadErr before rebuilding, got %v", a.loadErr)
	}
}

func TestAppQuitsBeforePage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/no/such/path", "test")
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Errorf("q before page should return Quit cmd, got nil")
	}
}

func TestAppCenterOverlayFallback(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/no/such/path", "test")
	if got := a.centerOverlay("hello"); got != "hello" {
		t.Errorf("zero-size fallback: want raw content, got %q", got)
	}
}

func TestAppCenterOverlayPlacesContent(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := bootApp(t)
	out := a.centerOverlay("X")
	if !strings.Contains(out, "X") {
		t.Errorf("centered output must contain content; got:\n%s", out)
	}
	if len(out) <= len("X") {
		t.Errorf("centered output should pad with whitespace; got len %d", len(out))
	}
}

func TestPageNameFromHitPath(t *testing.T) {
	idx := loadFixture(t)
	var sample, name string
	for _, p := range idx.Pages {
		sample, name = p.Path, p.Name
		break
	}
	if got := pageNameFromHitPath(idx, sample); got != name {
		t.Errorf("known path: want %q, got %q", name, got)
	}
	if got := pageNameFromHitPath(idx, "/totally/unknown/file.md"); got != "" {
		t.Errorf("unknown path: want \"\", got %q", got)
	}
}

func TestAppSearchDoneMsgRouting(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if a.search == nil {
		t.Fatalf("setup: search not open")
	}
	a.Update(searchDoneMsg{hits: []SearchHit{
		{FilePath: "/x", Line: 1, Context: "hello"},
	}})
	if len(a.search.hits) != 1 {
		t.Errorf("searchDoneMsg should populate hits, got %d", len(a.search.hits))
	}
}

func TestAppViewWithOverlay(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	v := a.View()
	if strings.Contains(v, "? help") {
		t.Errorf("overlay view should not render the page status bar; got:\n%s", v)
	}
	if !strings.Contains(v, "Open todos") {
		t.Errorf("overlay view should show todos title; got:\n%s", v)
	}
}
