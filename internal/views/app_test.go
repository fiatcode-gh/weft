package views

import (
	"path/filepath"
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
	a := New(abs)
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
