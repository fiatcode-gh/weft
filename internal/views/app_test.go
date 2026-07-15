package views

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	quietTerm(t)
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
	quietTerm(t)
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
	// arrange
	quietTerm(t)
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("- filler\n")
	}
	sb.WriteString("- mentions Hub down here\n")
	tmp, idx := writeGraph(t, map[string]string{
		"pages/Note.md": sb.String(),
		"pages/Hub.md":  "# Hub\n",
	})
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Hub", 80, 24)

	// act
	a.navigateHighlighting("Note", "Hub")

	// assert
	if a.page.Page() != "Note" {
		t.Fatalf("should navigate to Note; got %q", a.page.Page())
	}
	if len(a.page.result.Finds) == 0 {
		t.Errorf("destination should have highlighted finds for the bare Hub mention")
	}
}

func TestAppBacklinkUnlinkedEnterHighlights(t *testing.T) {
	// arrange
	quietTerm(t)
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH")
	}
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("- filler\n")
	}
	sb.WriteString("- a bare Topic mention\n")
	tmp, idx := writeGraph(t, map[string]string{
		"pages/Topic.md": "# Topic\n",
		"pages/Note.md":  sb.String(),
	})
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Topic", 80, 24)

	// act + assert
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
	// arrange — Alpha.md mentions its own name (must be excluded as self),
	// Note.md has one bare mention (kept) and one already-linked (dropped).
	tmp, idx := writeGraph(t, map[string]string{
		"pages/Alpha.md": "# Alpha\n",
		"pages/Note.md":  "a bare Alpha mention\nand a linked [[Alpha]]\n",
	})
	a := New(tmp, "test")
	a.idx = idx

	// act
	refs := a.unlinkedRefs("Alpha")

	// assert
	if len(refs) != 1 {
		t.Fatalf("want 1 unlinked ref (Note bare mention), got %d: %+v", len(refs), refs)
	}
	if refs[0].PageName != "Note" || refs[0].Line != 1 {
		t.Errorf("unexpected ref: %+v", refs[0])
	}
}

func TestAppLinkifyWritesAndRefreshes(t *testing.T) {
	quietTerm(t)
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	tmp, idx := writeGraph(t, map[string]string{
		"pages/Topic.md": "# Topic\n",
		"pages/Note.md":  "- a bare Topic mention\n",
	})
	notePath := filepath.Join(tmp, "pages", "Note.md")
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Topic", 80, 24)

	a.Update(key("b"))                      // open backlinks for Topic
	a.Update(tea.KeyMsg{Type: tea.KeyDown}) // move onto the unlinked Note row
	a.Update(key("l"))                      // open the confirm
	a.Update(key("y"))                      // confirm the write

	got, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "- a bare [[Topic]] mention\n"; string(got) != want {
		t.Fatalf("Note.md = %q, want %q", string(got), want)
	}
	// Panel refreshed: the mention is now a linked ref, so it has dropped out
	// of the unlinked list.
	bl, ok := a.active.(*Backlinks)
	if !ok {
		t.Fatalf("backlinks overlay should still be open after linkify; got %T", a.active)
	}
	for _, r := range bl.rows {
		if r.unl != nil {
			t.Errorf("linkified mention should no longer appear as unlinked: %+v", r.unl)
		}
	}
}

func TestAppLinkifyMentionGoneShowsError(t *testing.T) {
	quietTerm(t)
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	tmp, idx := writeGraph(t, map[string]string{
		"pages/Topic.md": "# Topic\n",
		"pages/Note.md":  "- a bare Topic mention\n",
	})
	notePath := filepath.Join(tmp, "pages", "Note.md")
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Topic", 80, 24)

	a.Update(key("b"))
	a.Update(tea.KeyMsg{Type: tea.KeyDown})
	a.Update(key("l"))
	// The mention vanishes from the file between detection and confirm.
	if err := os.WriteFile(notePath, []byte("- nothing here now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.Update(key("y"))

	bl, ok := a.active.(*Backlinks)
	if !ok {
		t.Fatalf("panel should stay open on error; got %T", a.active)
	}
	if bl.errMsg == "" {
		t.Error("a vanished mention should set an in-panel error message")
	}
	got, _ := os.ReadFile(notePath)
	if string(got) != "- nothing here now\n" {
		t.Errorf("file must be untouched when the mention is gone; got %q", string(got))
	}
}

// TestStaleIndexLoadedMsgIsDropped pins the fix for a real race: a sync-pull
// reindex and an R-triggered reindex can be in flight together, and the
// OLDER disk walk can deliver LAST. Without a generation tag, that stale
// result would silently overwrite the newer index.
func TestStaleIndexLoadedMsgIsDropped(t *testing.T) {
	// arrange — two reindexes in flight; the OLDER walk delivers LAST
	a := bootApp(t)
	oldCmd := a.buildIndexCmd()
	oldMsg := oldCmd().(indexLoadedMsg)
	newCmd := a.buildIndexCmd()
	newMsg := newCmd().(indexLoadedMsg)

	// act
	a.Update(newMsg)
	current := a.idx
	a.Update(oldMsg) // stale delivery

	// assert
	if a.idx != current {
		t.Fatal("stale indexLoadedMsg replaced the newer index")
	}
}
