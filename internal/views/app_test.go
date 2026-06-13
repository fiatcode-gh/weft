package views

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

func TestTodayJournalNameUsesNowFunc(t *testing.T) {
	a := New("/unused", "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC) }
	if got := a.todayJournalName(); got != "2026-05-23" {
		t.Errorf("todayJournalName: want 2026-05-23, got %q", got)
	}
}

// bootAppAt is like bootApp but pins App.nowFunc before the WindowSizeMsg so
// tryInitPage seeds history with a deterministic page name. Use for tests
// that exercise the . / < / > keys against the fixture.
func bootAppAt(t *testing.T, now time.Time) *App {
	t.Helper()
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	a := New(abs, "test")
	a.nowFunc = func() time.Time { return now }
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	a.Update(msg)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	return a
}

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

func TestAppNavigateFocusingLink(t *testing.T) {
	a := bootApp(t)
	// Navigate to kb/notes focusing on the [[Hub]] link it contains.
	a.navigateFocusingLink("kb/notes", "Hub")
	if got := a.page.Page(); got != "kb/notes" {
		t.Fatalf("navigateFocusingLink: want page kb/notes, got %q", got)
	}
	if a.page.Cursor() < 0 {
		t.Fatalf("navigateFocusingLink: cursor should be on the Hub link, got -1")
	}
	resolved, ok := a.idx.Resolve(a.page.FollowCursor())
	if !ok || resolved.Name != "Hub" {
		t.Errorf("navigateFocusingLink: cursor should resolve to Hub; FollowCursor()=%q", a.page.FollowCursor())
	}
	// The cursor is stored in the history entry so history navigation restores it.
	if a.hist[a.histIdx].cursor != a.page.Cursor() {
		t.Errorf("history entry cursor not persisted: hist=%d page=%d",
			a.hist[a.histIdx].cursor, a.page.Cursor())
	}
}

func TestAppNavigateHighlighting(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("- filler\n")
	}
	sb.WriteString("- mentions Hub down here\n")
	if err := os.WriteFile(filepath.Join(pages, "Note.md"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "Hub.md"), []byte("# Hub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(tmp)
	if err != nil {
		t.Fatal(err)
	}
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Hub", 80, 24)
	a.navigateHighlighting("Note", "Hub")
	if a.page.Page() != "Note" {
		t.Fatalf("should navigate to Note; got %q", a.page.Page())
	}
	if len(a.page.result.Finds) == 0 {
		t.Errorf("destination should have highlighted finds for the bare Hub mention")
	}
}

func TestAppBacklinkUnlinkedEnterHighlights(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH")
	}
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "Topic.md"), []byte("# Topic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("- filler\n")
	}
	sb.WriteString("- a bare Topic mention\n")
	if err := os.WriteFile(filepath.Join(pages, "Note.md"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(tmp)
	if err != nil {
		t.Fatal(err)
	}
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Topic", 80, 24)
	// Open backlinks for Topic; Note has an unlinked "Topic" mention.
	a.Update(key("b"))
	// The backlinks panel for "Topic": no linked refs, one unlinked (Note).
	// Move to the first selectable row and accept.
	a.Update(tea.KeyMsg{Type: tea.KeyDown})
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.page.Page() != "Note" {
		t.Fatalf("enter on the unlinked ref should navigate to Note; got %q", a.page.Page())
	}
	if len(a.page.result.Finds) == 0 {
		t.Errorf("destination should highlight the bare Topic mention; got 0 finds")
	}
}

func TestAppUnlinkedRefs(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	// Alpha.md mentions its own name (must be excluded as self),
	// Note.md has one bare mention (kept) and one already-linked (dropped).
	if err := os.WriteFile(filepath.Join(pages, "Alpha.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "Note.md"), []byte("a bare Alpha mention\nand a linked [[Alpha]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(tmp)
	if err != nil {
		t.Fatal(err)
	}
	a := New(tmp, "test")
	a.idx = idx

	refs := a.unlinkedRefs("Alpha")
	if len(refs) != 1 {
		t.Fatalf("want 1 unlinked ref (Note bare mention), got %d: %+v", len(refs), refs)
	}
	if refs[0].PageName != "Note" || refs[0].Line != 1 {
		t.Errorf("unexpected ref: %+v", refs[0])
	}
}
