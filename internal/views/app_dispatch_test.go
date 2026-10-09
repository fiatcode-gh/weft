package views

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestPageEdgeKeys(t *testing.T) {
	a := bootApp(t)
	// Navigate to Alpha so we have real content that can scroll. Boot
	// page (today's journal) is typically empty in the fixture.
	a.navigate("Alpha")

	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	// 'G' jumps to bottom.
	a.Update(key("G"))
	bottomOffset := a.page.Offset()
	if bottomOffset == 0 {
		t.Errorf("after G: expected non-zero offset, got 0")
	}

	// 'g' jumps back to top.
	a.Update(key("g"))
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

func TestAppLoadingSplashBeforePage(t *testing.T) {
	quietTerm(t)
	a := New("/nonexistent/before/build", "test")
	v := appText(a)
	if !strings.Contains(v, "weft") {
		t.Errorf("splash should contain title; got:\n%s", v)
	}
	if !strings.Contains(v, "Loading") {
		t.Errorf("splash should announce loading; got:\n%s", v)
	}
}

func TestAppErrorSplashOnIndexFailure(t *testing.T) {
	quietTerm(t)
	a := New("/some/graph", "test")
	a.Update(indexLoadedMsg{err: errors.New("synthetic build failure")})
	v := appText(a)
	if !strings.Contains(v, "failed to index") {
		t.Errorf("error splash should announce failure; got:\n%s", v)
	}
	if !strings.Contains(v, "R to retry") {
		t.Errorf("error splash should show retry hint; got:\n%s", v)
	}
}

func TestAppRetryFromErrorSplash(t *testing.T) {
	quietTerm(t)
	a := New("/some/graph", "test")
	a.Update(indexLoadedMsg{err: errors.New("synthetic build failure")})
	if a.loadErr == nil {
		t.Fatalf("setup: loadErr should be set")
	}

	_, cmd := a.Update(key("R"))
	if cmd == nil {
		t.Fatalf("R from error splash should return a retry cmd")
	}
	if a.loadErr != nil {
		t.Errorf("R should clear loadErr before rebuilding, got %v", a.loadErr)
	}
}

// TestAppMidSessionReindexFailureKeepsPage pins the parallel mid-session
// behaviour: when a reindex fails while the user already has a page on
// screen, the page must stay visible. The error should surface as a
// transient status hint, not a splash that replaces the working view.
// (The boot path, by contrast, does show the splash — covered by
// TestAppRetryFromErrorSplash above.)
func TestAppMidSessionReindexFailureKeepsPage(t *testing.T) {
	a := bootApp(t)
	bootPage := a.page.Page()
	// Simulate the R-then-fail path: the keypress schedules a reindex
	// (cmd is irrelevant — the test synthesises the response below). gen
	// must match a.indexGen (R just bumped it) or the generation guard
	// would drop this synthetic message as stale before it ever reaches
	// the failure-handling branch under test.
	a.Update(key("R"))
	a.Update(indexLoadedMsg{err: errors.New("disk full"), gen: a.indexGen})

	if !strings.Contains(a.hint, "disk full") {
		t.Errorf("mid-session reindex failure should surface a hint; got hint %q", a.hint)
	}
	view := appText(a)
	if strings.Contains(view, "failed to index") {
		t.Errorf("replaced the working page with the error splash:\n%s", view)
	}
	if !strings.Contains(view, bootPage) {
		t.Errorf("expected boot page %q in the view, got:\n%s", bootPage, view)
	}
}

func TestAppQuitsBeforePage(t *testing.T) {
	quietTerm(t)
	a := New("/no/such/path", "test")
	_, cmd := a.Update(key("q"))
	if cmd == nil {
		t.Errorf("q before page should return Quit cmd, got nil")
	}
}

func TestAppPageKeyDispatch(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5}) // scrollable

	a.Update(key("n"))
	if a.page.Cursor() < 0 {
		t.Errorf("n should advance cursor from -1, got %d", a.page.Cursor())
	}

	a.Update(key("N"))
	a.page.GotoTop()
	start := a.page.CursorRow()
	a.Update(key("j"))
	afterJ := a.page.CursorRow()
	if afterJ <= start {
		t.Fatalf("setup: j should advance the cursor from %d, got %d", start, afterJ)
	}
	// The window is three rows: the cursor reached its bottom edge, so a second j
	// scrolls.
	a.Update(key("j"))
	if a.page.Offset() == 0 {
		t.Errorf("j past the window edge should scroll, offset 0 (cursor row %d)", a.page.CursorRow())
	}

	a.Update(key("k"))
	if a.page.CursorRow() != afterJ {
		t.Errorf("k should return the cursor to row %d, got %d", afterJ, a.page.CursorRow())
	}

	a.page.GotoTop()
	a.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	afterCtrlD := a.page.CursorRow()
	if afterCtrlD <= start || a.page.Offset() == 0 {
		t.Fatalf("setup: ctrl+d should move cursor (%d, from %d) and top (%d)", afterCtrlD, start, a.page.Offset())
	}
	a.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if a.page.CursorRow() >= afterCtrlD || a.page.Offset() != 0 {
		t.Errorf("ctrl+u should retreat from row %d, got row %d offset %d", afterCtrlD, a.page.CursorRow(), a.page.Offset())
	}
}

func TestAppEnterFollowsLink(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	a.page.CycleLink(+1)
	target := a.page.FollowCursor()
	if target == "" {
		t.Fatalf("setup: no link target available")
	}

	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.page.Page() != target {
		t.Errorf("enter should navigate to %q, got %q", target, a.page.Page())
	}
}

func TestAppReindexFromPage(t *testing.T) {
	a := bootApp(t)
	_, cmd := a.Update(key("R"))
	if cmd == nil {
		t.Errorf("R from page mode should return a reindex cmd")
	}
}

// TestStatusBarTruncatesLongLeft asserts that a wide page name + meta
// don't overflow the terminal width — the left segment is clamped with
// an ellipsis so the bar fits on one line.
func TestStatusBarTruncatesLongLeft(t *testing.T) {
	a := bootApp(t)
	// Width 20 is narrower than left ("Alpha · N links") plus right
	// ("? help"), forcing the left clamp; assert no rendered line exceeds
	// the terminal width.
	a.navigate("Alpha")
	a.Update(tea.WindowSizeMsg{Width: 20, Height: 24})

	bar := plain(a.statusBar())
	for _, line := range strings.Split(bar, "\n") {
		if w := runewidthLen(line); w > 20 {
			t.Errorf("status bar line exceeds width 20: w=%d, line=%q", w, line)
		}
	}
	// Even with the clamp, the right segment (which carries the help
	// hint) must survive — it's the user's only on-screen reminder of
	// how to open help.
	if !strings.Contains(bar, "? help") {
		t.Errorf("clamped bar dropped the close hint; got:\n%s", bar)
	}
}

// runewidthLen counts visible cells in a single line, ignoring nothing
// (callers strip ANSI first, with plain).
func runewidthLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func TestPeriodJumpsToTodayJournal(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)})
	// Boot lands on today's journal (2026-05-23) per tryInitPage seeding.
	// Navigate elsewhere first so we can verify . actually moves us.
	a.navigate("Alpha")
	startHistLen := len(a.hist)

	a.Update(key("."))

	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after .: want page 2026-05-23, got %q", got)
	}
	if got := len(a.hist); got != startHistLen+1 {
		t.Errorf("history len: want %d, got %d", startHistLen+1, got)
	}
	if a.hint != "" {
		t.Errorf("hint should be empty on successful jump, got %q", a.hint)
	}
}

func TestPeriodOnAbsentTodayCreatesAndNavigates(t *testing.T) {
	// 2026-06-15 has no journal in the fixture; `.` must create the file
	// and navigate to it.
	a := bootApp(t, bootConfig{now: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)})
	a.navigate("Alpha")

	a.Update(key("."))

	if got := a.page.Page(); got != "2026-06-15" {
		t.Errorf("after . on absent today: want page 2026-06-15, got %q", got)
	}
	if _, ok := a.idx.ByName["2026-06-15"]; !ok {
		t.Errorf("after . on absent today: 2026-06-15 should be in idx.ByName")
	}
	// The newly created file should be on disk in the throwaway clone
	// (t.TempDir() cleans it up automatically — no manual cleanup needed).
	path := filepath.Join(a.graphPath, "journals", "2026_06_15.md")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("after . on absent today: journal file should exist; stat err=%v", err)
	}
}

func TestHintClearsOnNextKey(t *testing.T) {
	// `.` on a missing journal creates+navigates rather than hinting, so
	// trigger a hint via `<` at the oldest journal ("no earlier journal").
	a := bootApp(t, bootConfig{now: time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)})
	a.Update(key("<"))
	if a.hint == "" {
		t.Fatal("setup: expected hint to be set by < at oldest")
	}
	// Any subsequent key clears the hint at the top of Update.
	a.Update(key("j"))
	if a.hint != "" {
		t.Errorf("hint should clear on next key, got %q", a.hint)
	}
}

func TestHintExpiresOnTick(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)})
	a.Update(key("<"))
	if a.hint == "" {
		t.Fatal("setup: expected < to set a hint")
	}
	gen := a.hintGen
	a.Update(hintExpireMsg{gen: gen})
	if a.hint != "" {
		t.Errorf("hint should clear after matching tick; got %q", a.hint)
	}
}

func TestStaleHintTickIgnored(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)})
	a.Update(key("<"))
	staleGen := a.hintGen
	// Second < clears the hint via the top-of-KeyPressMsg sweep, then re-sets it
	// with a fresh generation. The tick scheduled by the first < is now stale.
	a.Update(key("<"))
	if a.hint == "" {
		t.Fatal("setup: expected second < to set a new hint")
	}
	current := a.hint
	a.Update(hintExpireMsg{gen: staleGen})
	if a.hint != current {
		t.Errorf("stale tick should not clear current hint; want %q, got %q", current, a.hint)
	}
}

func TestPeriodIdempotentOnTodayJournal(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)})
	// Boot already lands on today's journal.
	if got := a.page.Page(); got != "2026-05-23" {
		t.Fatalf("setup: want boot page 2026-05-23, got %q", got)
	}
	startHistLen := len(a.hist)
	a.Update(key("."))
	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after . on today: want 2026-05-23, got %q", got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("after . on today: history grew from %d to %d", startHistLen, got)
	}
}

func TestPrevJournalWalksBackwards(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)})
	// Boot lands on 2026-05-24.
	a.Update(key("<"))
	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after <: want 2026-05-23, got %q", got)
	}
	if a.hint != "" {
		t.Errorf("hint should be empty on successful walk, got %q", a.hint)
	}
}

func TestNextJournalWalksForward(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)})
	a.Update(key(">"))
	if got := a.page.Page(); got != "2026-05-24" {
		t.Errorf("after >: want 2026-05-24, got %q", got)
	}
}

func TestPrevJournalSkipsGapDays(t *testing.T) {
	// Fixture has 2026-04-20 then 2026-03-15 — large gap. < from 04-20 lands
	// on 03-15, skipping the missing calendar days in between.
	a := bootApp(t, bootConfig{now: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)})
	a.Update(key("<"))
	if got := a.page.Page(); got != "2026-03-15" {
		t.Errorf("after < across gap: want 2026-03-15, got %q", got)
	}
}

func TestPrevJournalAtOldestShowsHint(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)})
	startPage := a.page.Page()
	startHistLen := len(a.hist)
	a.Update(key("<"))
	if got := a.page.Page(); got != startPage {
		t.Errorf("< at oldest: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("< at oldest grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "no earlier journal" {
		t.Errorf("hint: want %q, got %q", "no earlier journal", a.hint)
	}
}

func TestNextJournalAtNewestShowsHint(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)})
	startPage := a.page.Page()
	startHistLen := len(a.hist)
	a.Update(key(">"))
	if got := a.page.Page(); got != startPage {
		t.Errorf("> at newest: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("> at newest grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "no later journal" {
		t.Errorf("hint: want %q, got %q", "no later journal", a.hint)
	}
}

func TestPrevJournalFromPhantomToday(t *testing.T) {
	// 2026-06-15 has no fixture journal; it's phantom-today. < should walk
	// to the newest existing fixture journal (2026-05-25).
	a := bootApp(t, bootConfig{now: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)})
	if got := a.page.Page(); got != "2026-06-15" {
		t.Fatalf("setup: want boot page 2026-06-15, got %q", got)
	}
	a.Update(key("<"))
	if got := a.page.Page(); got != "2026-05-25" {
		t.Errorf("after < from phantom today: want 2026-05-25, got %q", got)
	}
	if a.hint != "" {
		t.Errorf("hint should be empty on successful walk, got %q", a.hint)
	}
}

func TestNextJournalFromPhantomTodayShowsHint(t *testing.T) {
	// 2026-06-15 is phantom-today; no fixture journal is later. > should
	// surface "no later journal" without navigating.
	a := bootApp(t, bootConfig{now: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)})
	startPage := a.page.Page()
	startHistLen := len(a.hist)
	a.Update(key(">"))
	if got := a.page.Page(); got != startPage {
		t.Errorf("> from phantom today (no later): page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("> from phantom today: history grew from %d to %d", startHistLen, got)
	}
	if a.hint != "no later journal" {
		t.Errorf("hint: want %q, got %q", "no later journal", a.hint)
	}
}

func TestPrevNextInertOutsideJournalContext(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)})
	a.navigate("Alpha")
	startPage := a.page.Page()
	startHistLen := len(a.hist)

	a.Update(key("<"))
	a.Update(key(">"))

	if got := a.page.Page(); got != startPage {
		t.Errorf("<,> outside journal context: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("<,> outside journal context grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "" {
		t.Errorf("<,> outside journal context: hint should be empty, got %q", a.hint)
	}
}

// TestEditBootstrapRebuildsPageView verifies that pressing `e` on a cold-start
// today's-journal page (no file yet) reindexes AND rebinds the PageView to the
// fresh index — not just a.idx. Regression guard for the stale-a.page bug.
//
// Uses a t.TempDir-based graph (mirroring TestEditKey_ColdStartCreatesTodayJournal)
// rather than the shared fixture so creating a file is safe and idempotent.
func TestEditBootstrapRebuildsPageView(t *testing.T) {
	quietTerm(t)
	t.Setenv("EDITOR", "true") // no-op editor that exits 0 immediately
	// An anchor page, but no journal file — today's journal is missing on disk.
	dir, _ := writeGraph(t, map[string]string{"pages/Anchor.md": "anchor\n"})

	a := New(dir, "test")
	today := "2026-05-26"
	a.nowFunc = func() time.Time { return time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC) }
	cmd := a.Init()
	if cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	if a.page.Page() != today {
		t.Fatalf("precondition: boot page should be %q, got %q", today, a.page.Page())
	}
	if _, ok := a.idx.ByName[today]; ok {
		t.Fatalf("precondition: today's journal should NOT be in index; file doesn't exist")
	}

	// Press `E` directly — cold-start bootstrap ($EDITOR path).
	_, cmd = a.Update(key("E"))
	if cmd == nil {
		t.Fatal("E on cold-start should return a non-nil editor cmd; got nil (hint: " + a.hint + ")")
	}

	if _, ok := a.idx.ByName[today]; !ok {
		t.Fatalf("index should contain freshly-created journal %q", today)
	}
	// The PageView must resolve against the rebuilt index: its page is today's
	// journal and that page is now present in the index it holds.
	if a.page.idx != a.idx {
		t.Errorf("PageView still bound to stale index after E-bootstrap")
	}
	if a.page.Page() != today {
		t.Errorf("PageView page: want %q, got %q", today, a.page.Page())
	}
}

// TestSaveFailureKeepsEditorAndBuffer pins the highest-consequence untested
// flow in the Ctrl+S handler (app.go's tea.KeyPressMsg case, res.Save branch):
// when the guarded write (edit.WriteFileIfUnchanged) fails, the in-app editor must stay open with the
// buffer intact and the error surfaced — never torn down and never losing
// unsaved work. The save is made to fail deterministically by revoking
// write permission on the page's directory, which makes writeFile's
// os.CreateTemp(dir, ...) fail before anything is touched on disk.
func TestSaveFailureKeepsEditorAndBuffer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a 0o555 directory does not block writes")
	}
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/A.md": "- start\n"})
	a := New(dir, "test")
	cmd := a.Init()
	if cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("A")

	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("precondition: editor did not open")
	}
	a.Update(key("x"))
	content := a.editor.Content()

	// Make the save fail: the page directory becomes read-only, so
	// the guarded write's temp-file creation in that directory fails.
	pages := filepath.Join(dir, "pages")
	if err := os.Chmod(pages, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(pages, 0o755) })

	a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	if a.editor == nil {
		t.Fatal("failed save tore down the editor (buffer lost)")
	}
	if a.editor.Content() != content {
		t.Fatalf("buffer changed across failed save: got %q, want %q", a.editor.Content(), content)
	}
	if a.editor.errMsg == "" {
		t.Fatal("save error not surfaced on the editor (errMsg empty)")
	}
	if a.editor.saved {
		t.Error("editor reports saved=true after a failed save")
	}
}

// TestEditCurrentRecreatesDeletedFile pins the defensive branch in
// editCurrent that handles a page whose file vanished between BuildIndex
// and the `e` keypress (e.g. an external tool deleted it mid-session): the
// stub is recreated on disk before the editor is handed off, rather than
// failing or handing the editor a nonexistent path.
func TestEditCurrentRecreatesDeletedFile(t *testing.T) {
	// arrange — indexed page whose file vanished before E
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/A.md": "- x\n"})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("A")
	fake, err := filepath.Abs("../../testdata/fake-editor.sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", fake)
	path := filepath.Join(dir, "pages", "A.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// act — editCurrent must recreate the stub before handing off
	cmd := a.editCurrent()

	// assert
	if cmd == nil {
		t.Fatalf("editCurrent returned nil cmd (hint: %q)", a.hint)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("deleted file was not recreated: %v", err)
	}
}

// TestEditCurrentSurfacesEditorResolutionFailure pins the defensive branch
// in editCurrent where edit.Resolve fails to find any usable editor: a
// "cannot resolve editor" hint is surfaced instead of a panic or a silently
// dropped keypress.
//
// edit.Resolve falls back to the absolute path /usr/bin/vi, and
// exec.LookPath resolves absolute paths by stat-ing them directly —
// it does not consult $PATH for those, so clearing $PATH cannot hide
// /usr/bin/vi from the fallback. On a machine that has vi installed at
// that exact path, resolution failure cannot be forced portably, so this
// test is skipped there rather than faked.
func TestEditCurrentSurfacesEditorResolutionFailure(t *testing.T) {
	if _, err := os.Stat("/usr/bin/vi"); err == nil {
		t.Skip("/usr/bin/vi exists on this machine; edit.Resolve's fallback " +
			"will always succeed via LookPath regardless of $PATH, so " +
			"resolution failure cannot be forced portably here")
	}

	// arrange — no resolvable editor anywhere in the chain
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/A.md": "- x\n"})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("A")
	t.Setenv("VISUAL", "/nonexistent/editor-binary")
	t.Setenv("EDITOR", "/nonexistent/editor-binary")
	t.Setenv("PATH", t.TempDir()) // hides /usr/bin/vi from LookPath's fallback

	// act
	_ = a.editCurrent()

	// assert — setHint stamps a.hint synchronously; the cmd it returns is
	// only the expiry tick (executing it would sleep hintTTL and then
	// CLEAR the hint under assertion)
	if !strings.Contains(a.hint, "cannot resolve editor") {
		t.Fatalf("hint = %q, want editor-resolution failure", a.hint)
	}
}

// TestEditorExitUnchangedMtimeSkipsReindex pins the index-invalidation gate
// in the editorExitedMsg handler: when the editor exits without having
// touched the file (mtime unchanged from the snapshot taken before
// hand-off), only the sync-status probe runs — no reindex is scheduled.
// indexGen is bumped synchronously inside buildIndexCmd, so it's the
// observable for "was a reindex scheduled".
func TestEditorExitUnchangedMtimeSkipsReindex(t *testing.T) {
	// arrange
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/A.md": "- x\n"})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("A")
	path := filepath.Join(dir, "pages", "A.md")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	genBefore := a.indexGen

	// act — editor exited without touching the file
	a.Update(editorExitedMsg{path: path, t0: info.ModTime()})

	// assert — no reindex was scheduled (indexGen is bumped
	// synchronously inside buildIndexCmd, so it's the observable)
	if a.indexGen != genBefore {
		t.Fatal("unchanged-mtime editor exit triggered a reindex")
	}
}
