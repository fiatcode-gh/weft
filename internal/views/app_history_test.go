package views

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

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
	a.Update(key("["))
	if got := a.page.Page(); got != "Alpha" {
		t.Errorf("after [: want Alpha, got %q", got)
	}

	// '[' again -> startPage
	a.Update(key("["))
	if got := a.page.Page(); got != startPage {
		t.Errorf("after second [: want %q, got %q", startPage, got)
	}

	// ']' -> Alpha
	a.Update(key("]"))
	if got := a.page.Page(); got != "Alpha" {
		t.Errorf("after ]: want Alpha, got %q", got)
	}
}

func TestHistoryBackAtStartNoop(t *testing.T) {
	a := bootApp(t)
	startPage := a.page.Page()
	startIdx := a.histIdx

	a.Update(key("["))

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

	a.Update(key("]"))

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
	a.Update(key("["))
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
	a.Update(key("]"))
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
	a.Update(key("["))
	if a.page.Page() != "Alpha" {
		t.Fatalf("after [: want Alpha, got %q", a.page.Page())
	}
	if got := a.page.Cursor(); got != wantCursor {
		t.Errorf("restored cursor: want %d, got %d", wantCursor, got)
	}
}

// TestAppTodosDeepLinkScrollsToBullet exercises the end-to-end deep-link:
// open the Todos dashboard, filter to TODO, hit Enter, and verify the
// app navigates to the bullet's page and records the bullet's open-todo
// ordinal in the new history entry (which is what PageView.ScrollToTask reads).
func TestAppTodosDeepLinkScrollsToBullet(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)})
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open todos.
	a.Update(key("T"))
	// Cycle to TODO filter; markerCycle = ["", "TODO", "LATER", "DOING", "WAITING"]
	// so one "t" press from "" lands on "TODO".
	a.Update(key("t"))
	// Select the first row.
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// The first open TODO in the fixture is on 2026-05-24, line 2
	// ("TODO Ship the TUI MVP"). Sorting is by page name then line number.
	if got := a.page.Page(); got != "2026-05-24" {
		t.Fatalf("after deep-link: page want 2026-05-24, got %q", got)
	}

	// Strong pin: the deep-link target is recorded in the new history
	// entry. This is what navigateToTask populates and what ScrollToTask
	// reads. The first open TODO on 2026-05-24 ("Ship the TUI MVP") is
	// ordinal 0 on that page.
	last := a.hist[len(a.hist)-1]
	if last.taskOrdinal != 0 {
		t.Errorf("history entry taskOrdinal: want 0, got %d (entry: %+v)", last.taskOrdinal, last)
	}

	// Smoke check: the bullet text is rendered on screen. With the
	// fixture's short page the bullet is always visible regardless of
	// deep-link, so this only catches "we navigated to the wrong page".
	view := appText(a)
	if !strings.Contains(view, "Ship the TUI MVP") {
		t.Errorf("expected 'Ship the TUI MVP' on screen, got:\n%s", view)
	}
}

func TestHistoryRestoresCursorRow(t *testing.T) {
	quietTerm(t)
	a := newApp(t, map[string]string{
		"pages/A.md": bulletsOf("a", 60),
		"pages/B.md": bulletsOf("b", 60),
	}, 80, 12)
	a.navigate("A")
	for range 15 {
		a.Update(key("j"))
	}
	row, off := a.page.CursorRow(), a.page.Offset()
	if off == 0 {
		t.Fatalf("setup: the window must have scrolled, offset %d", off)
	}
	a.navigate("B")
	if got := a.hist[a.histIdx-1].row; got != row {
		t.Fatalf("departing entry stored row %d, want %d", got, row)
	}
	bRow, bOff := a.page.CursorRow(), a.page.Offset()

	a.Update(key("["))
	if a.page.Page() != "A" || a.page.CursorRow() != row || a.page.Offset() != off {
		t.Fatalf("[ landed on %q row %d offset %d, want A row %d offset %d",
			a.page.Page(), a.page.CursorRow(), a.page.Offset(), row, off)
	}
	a.Update(key("]"))
	if a.page.Page() != "B" || a.page.CursorRow() != bRow || a.page.Offset() != bOff {
		t.Fatalf("] landed on %q row %d offset %d, want B row %d offset %d",
			a.page.Page(), a.page.CursorRow(), a.page.Offset(), bRow, bOff)
	}
}
