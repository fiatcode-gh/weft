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

type bootConfig struct {
	files map[string]string
	now   time.Time
}

// bootApp loads a test graph and initializes PageView through the production
// boot order. With no config it clones the shared fixture.
func bootApp(t *testing.T, configs ...bootConfig) *App {
	t.Helper()
	quietTerm(t)
	if len(configs) > 1 {
		t.Fatalf("bootApp accepts at most one config, got %d", len(configs))
	}
	cfg := bootConfig{}
	if len(configs) == 1 {
		cfg = configs[0]
	}

	var graphPath string
	if cfg.files != nil {
		graphPath, _ = writeGraph(t, cfg.files)
	} else {
		graphPath = cloneFixtureGraph(t)
	}
	a := New(graphPath, "test")
	if !cfg.now.IsZero() {
		now := cfg.now
		a.nowFunc = func() time.Time { return now }
	}
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	a.Update(cmd())
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
	// App.linkify's fail closure supplies the "linkify failed: " framing —
	// the panel itself renders errMsg verbatim (see Finding 2: the busy
	// message from blockIfSyncing must NOT get this framing).
	if !strings.Contains(bl.errMsg, "linkify failed") {
		t.Errorf("errMsg = %q, want the linkify-failed prefix from App.linkify's fail closure", bl.errMsg)
	}
	got, _ := os.ReadFile(notePath)
	if string(got) != "- nothing here now\n" {
		t.Errorf("file must be untouched when the mention is gone; got %q", string(got))
	}
}

// drainCmds delivers the indexLoadedMsg produced by cmd — running any
// tea.Batch children concurrently via drainFor — directly into a.Update, the
// same way the real Bubble Tea runtime feeds an async result back into the
// model. Reuses drainFor's batch-draining idiom rather than reimplementing it.
func drainCmds(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	msg := drainFor[indexLoadedMsg](t, cmd)
	a.Update(msg)
}

// TestReindexPreservesScrollPosition pins the fix for a real regression: R
// rebuilds PageView from scratch to pick up the freshly reindexed links/todos,
// but a from-scratch NewPageView starts at offset 0 — silently discarding the
// user's scroll position on every reindex.
func TestReindexPreservesScrollPosition(t *testing.T) {
	// arrange — long page, scrolled deep
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{
		"pages/Long.md": strings.Repeat("- line\n", 80),
	})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	a.navigate("Long")
	for i := 0; i < 30; i++ {
		a.page.LineDown()
	}
	want := a.page.Offset()
	if want == 0 {
		t.Fatal("precondition: page not scrolled")
	}

	// act — R reindex, driven synchronously
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	drainCmds(t, a, cmd) // deliver the batch's indexLoadedMsg

	// assert
	if got := a.page.Offset(); got != want {
		t.Fatalf("offset after R = %d, want %d", got, want)
	}
}

// TestEditorDiscardExitPreservesScrollPosition pins the same regression on the
// editor discard-exit path: pressing e then a clean esc (no unsaved changes,
// no confirm) rebuilds PageView for the same page and must keep the reader's
// place instead of resetting to the top.
func TestEditorDiscardExitPreservesScrollPosition(t *testing.T) {
	// arrange — same long page, scrolled, then e → esc (clean buffer)
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{
		"pages/Long.md": strings.Repeat("- line\n", 80),
	})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	a.navigate("Long")
	for i := 0; i < 30; i++ {
		a.page.LineDown()
	}
	want := a.page.Offset()
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if a.editor == nil {
		t.Fatal("precondition: editor did not open")
	}

	// act — clean esc discards without confirm and rebuilds the page view
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// assert
	if a.editor != nil {
		t.Fatal("editor did not close on clean esc")
	}
	if got := a.page.Offset(); got != want {
		t.Fatalf("offset after e→esc = %d, want %d", got, want)
	}
}

// reindex() — the synchronous path used by linkify and journal-create —
// rebuilt the PageView from scratch, discarding scroll offset and link
// cursor: after y+Esc the page jumped to the top. Sync twin of
// TestReindexPreservesScrollPosition, which pins the async path.
func TestSyncReindexPreservesScrollPosition(t *testing.T) {
	// arrange
	quietTerm(t)
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/Long.md": strings.Repeat("- line\n", 80),
	}})
	a.navigate("Long")
	for i := 0; i < 30; i++ {
		a.page.LineDown()
	}
	want := a.page.Offset()
	if want == 0 {
		t.Fatal("arrange failed: page did not scroll")
	}

	// act
	if err := a.reindex(); err != nil {
		t.Fatal(err)
	}

	// assert
	if got := a.page.Offset(); got != want {
		t.Fatalf("offset after sync reindex = %d, want %d", got, want)
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

// Index warnings must reach the user: hinted in the status bar and
// appended to the debug log (stderr is invisible under the alt-screen).
func TestIndexWarningsSurfaceAsHintAndLog(t *testing.T) {
	// arrange
	quietTerm(t)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/Home.md":       "- hello\n",
		"pages/sub/Nested.md": "- nested\n", // pages/sub/ → one subdir warning
	}})

	// assert — boot indexing already ran inside bootApp
	if !strings.Contains(a.hint, "indexed with 1 warning — see ") {
		t.Fatalf("hint = %q, want indexed-with-warnings hint", a.hint)
	}
	logBytes, err := os.ReadFile(filepath.Join(cache, "weft", "weft.log"))
	if err != nil {
		t.Fatalf("expected warnings appended to the debug log: %v", err)
	}
	if !strings.Contains(string(logBytes), "skipping subdirectory") {
		t.Fatalf("log missing warning detail, got:\n%s", logBytes)
	}
}

// TestStandingIndexWarningsDoNotRenag pins the fix for a standing warning
// (an intentional pages/sub/, a long-lived case collision) re-nagging on
// every reindex: boot already logged and hinted the one warning above; a
// second reindex (R) that turns up the identical warning set must not hint
// "indexed with" again or append a second log line.
func TestStandingIndexWarningsDoNotRenag(t *testing.T) {
	// arrange — reuse the boot state above: one warning already surfaced
	quietTerm(t)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/Home.md":       "- hello\n",
		"pages/sub/Nested.md": "- nested\n", // pages/sub/ → one subdir warning
	}})
	if !strings.Contains(a.hint, "indexed with 1 warning — see ") {
		t.Fatalf("precondition: hint = %q, want indexed-with-warnings hint", a.hint)
	}
	a.hint = ""

	// act — R reindexes; the warning set is unchanged (still just pages/sub/)
	_, cmd := a.Update(key("R"))
	drainCmds(t, a, cmd)

	// assert — no re-nag: the hint is whatever R sets ("⟳ reindexing…" then
	// cleared/replaced), never the indexed-with-warnings hint again
	if strings.Contains(a.hint, "indexed with") {
		t.Fatalf("hint after standing-warning reindex = %q, want no re-nag", a.hint)
	}
	logBytes, err := os.ReadFile(filepath.Join(cache, "weft", "weft.log"))
	if err != nil {
		t.Fatalf("expected warnings appended to the debug log: %v", err)
	}
	if got := strings.Count(string(logBytes), "skipping subdirectory"); got != 1 {
		t.Fatalf("log has %d 'skipping subdirectory' lines, want exactly 1:\n%s", got, logBytes)
	}
}
