package views

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
	syncpkg "github.com/fiatcode-gh/weft/v2/internal/sync"
)

// pressE sends the e key to a booted app and returns the resulting cmd.
func pressE(t *testing.T, a *App) tea.Cmd {
	t.Helper()
	_, cmd := a.Update(key("e"))
	return cmd
}

// pressShiftE sends the E key (the $EDITOR escape hatch).
func pressShiftE(t *testing.T, a *App) tea.Cmd {
	t.Helper()
	_, cmd := a.Update(key("E"))
	return cmd
}

func TestE_EntersInAppEditor(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	pressE(t, a)
	if a.editor == nil {
		t.Fatalf("e should open the in-app editor")
	}
	if a.editor.pageName != "Alpha" {
		t.Errorf("editor page: got %q, want Alpha", a.editor.pageName)
	}
	if a.editor.Content() == "\n" || a.editor.Content() == "" {
		t.Errorf("editor should load Alpha's content; got %q", a.editor.Content())
	}
}

func TestShiftE_DispatchesToEditorProcess(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	if cmd := pressShiftE(t, a); cmd == nil {
		t.Errorf("E should return a tea.ExecProcess cmd; got nil")
	}
	if a.editor != nil {
		t.Errorf("E must not open the in-app editor")
	}
}

// TestEditorExitedMsg_NoReindexOnNoChange constructs the message that
// tea.ExecProcess would yield when the child exits cleanly without
// changing the file. The handler must not reindex, but it still refreshes
// the sync indicator (editCurrent may have created a journal stub), so the
// returned cmd is a status probe — not a buildIndexCmd.
func TestEditorExitedMsg_NoReindexOnNoChange(t *testing.T) {
	// Arrange: an existing fixture file, with t0 == its current mtime so the
	// handler takes the "unchanged" branch. Stub the probe to avoid git.
	a := bootApp(t)
	a.statusProbe = func(string) (syncpkg.WorktreeStatus, error) { return syncpkg.WorktreeStatus{}, nil }
	path := filepath.Join(a.graphPath, "pages", "Alpha.md")
	t0, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat Alpha.md: %v", err)
	}

	// Act
	_, cmd := a.Update(editorExitedMsg{path: path, t0: t0.ModTime(), err: nil})

	// Assert: a probe, not a reindex. drainFor fatals if the cmd produced
	// anything other than a statusProbedMsg (e.g. an indexLoadedMsg).
	drainFor[statusProbedMsg](t, cmd)
}

// TestEditorExitedMsg_TriggersReindexOnMtimeChange: the file's mtime
// has advanced; the handler should return a non-nil cmd (the
// buildIndexCmd).
func TestEditorExitedMsg_TriggersReindexOnMtimeChange(t *testing.T) {
	a := bootApp(t)
	path := filepath.Join(a.graphPath, "pages", "Alpha.md")

	// Pick a t0 well before the real mtime so the comparison advances.
	t0 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	_, cmd := a.Update(editorExitedMsg{path: path, t0: t0, err: nil})
	if cmd == nil {
		t.Errorf("mtime change should trigger reindex; got nil cmd")
	}
}

// TestEditorExitedMsg_NoReindexOnDelete: post-exit stat returns
// ENOENT — the user deleted the file in the editor. No reindex, but the
// indicator is still refreshed (the delete itself dirties the worktree).
func TestEditorExitedMsg_NoReindexOnDelete(t *testing.T) {
	// Arrange: a path that doesn't exist, so the post-exit stat returns ENOENT.
	a := bootApp(t)
	a.statusProbe = func(string) (syncpkg.WorktreeStatus, error) { return syncpkg.WorktreeStatus{}, nil }
	path := filepath.Join(t.TempDir(), "deleted.md")

	// Act
	_, cmd := a.Update(editorExitedMsg{path: path, t0: time.Time{}, err: nil})

	// Assert: a probe, not a reindex.
	drainFor[statusProbedMsg](t, cmd)
}

// TestEditorExitedMsg_HintOnError: non-nil err from the editor (e.g.
// :cq in vim, or signal). The handler should return a setHint cmd
// (non-nil because setHint schedules a tea.Tick).
func TestEditorExitedMsg_HintOnError(t *testing.T) {
	a := bootApp(t)
	_, cmd := a.Update(editorExitedMsg{path: "/nonexistent", t0: time.Time{}, err: errors.New("editor exited 7")})
	if cmd == nil {
		t.Errorf("editor error should surface a hint cmd; got nil")
	}
}

// TestShiftE_ColdStartCreatesTodayJournal: cold-start scenario.
// The user boots weft on a day with no journal file, lands on
// today, and presses `E` directly (without first pressing `.`).
// editCurrent must lazy-create the journal, reindex, and return a
// non-nil editor cmd. This is the case the user reported as broken
// when the create logic was tied to `.` only.
func TestShiftE_ColdStartCreatesTodayJournal(t *testing.T) {
	quietTerm(t)
	// An anchor page, but no journal file — today's journal is missing on disk.
	dir, _ := writeGraph(t, map[string]string{"pages/Anchor.md": "anchor\n"})
	a := New(dir, "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }
	cmd := a.Init()
	if cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	today := "2026-06-05"
	if a.page.Page() != today {
		t.Fatalf("precondition: boot page should be %q, got %q", today, a.page.Page())
	}
	if _, ok := a.idx.ByName[today]; ok {
		t.Fatalf("precondition: today's journal should NOT be in index; file doesn't exist")
	}
	journalPath := filepath.Join(dir, "journals", graph.FilenameFromPageName(today))
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: journal file should not exist; stat err=%v", err)
	}

	// Press `E` directly. No `.` first. The handler must bootstrap.
	cmd = pressShiftE(t, a)
	if cmd == nil {
		t.Fatalf("E on cold-start should return a non-nil editor cmd; got nil")
	}

	// File should now exist, empty, 0o644. The handler creates it
	// synchronously before returning the cmd.
	info, err := os.Stat(journalPath)
	if err != nil {
		t.Fatalf("after E on cold-start: journal file should exist; stat err=%v", err)
	}
	if info.Size() != 0 {
		t.Errorf("after E on cold-start: journal should be empty; got %d bytes", info.Size())
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("after E on cold-start: journal mode: want 0o644, got %v", info.Mode().Perm())
	}
	// And the index should now know about it.
	if _, ok := a.idx.ByName[today]; !ok {
		t.Errorf("after E on cold-start: %q should be in idx.ByName", today)
	}
}

func TestE_ColdStart_DefersCreation(t *testing.T) {
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/Anchor.md": "anchor\n"})
	a := New(dir, "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }
	if cmd := a.Init(); cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	today := "2026-06-05"
	journalPath := filepath.Join(dir, "journals", graph.FilenameFromPageName(today))

	pressE(t, a)
	if a.editor == nil {
		t.Fatalf("e on cold-start should open the editor")
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Errorf("e must NOT create the journal file before save; stat err=%v", err)
	}

	a.Update(key("h"))
	a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if _, err := os.Stat(journalPath); err != nil {
		t.Errorf("ctrl+s should create the journal file; stat err=%v", err)
	}
}

// TestDotKey_CreatesMissingTodayJournal_ThenEditReachable: end-to-end
// for the `.`-then-`e` flow. Boot with a journal file absent, press
// `.` to create-and-navigate, then press `e` to verify the page is
// now in the index and the editor cmd fires. Same outcome as the
// cold-start test, different path.
func TestDotKey_CreatesMissingTodayJournal_ThenEditReachable(t *testing.T) {
	quietTerm(t)
	// An anchor page, but no journal file — today's journal is missing on disk.
	dir, _ := writeGraph(t, map[string]string{"pages/Anchor.md": "anchor\n"})
	a := New(dir, "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }
	cmd := a.Init()
	if cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	today := "2026-06-05"
	journalPath := filepath.Join(dir, "journals", graph.FilenameFromPageName(today))
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: journal file should not exist; stat err=%v", err)
	}

	// Press `.`. This should create the journal file, reindex, and
	// land on today's journal.
	a.Update(key("."))
	if a.page.Page() != today {
		t.Errorf("after . on absent today: want page %q, got %q", today, a.page.Page())
	}
	if _, ok := a.idx.ByName[today]; !ok {
		t.Errorf("after . on absent today: %q should be in idx.ByName", today)
	}
	info, err := os.Stat(journalPath)
	if err != nil {
		t.Fatalf("after . on absent today: journal file should exist; stat err=%v", err)
	}
	if info.Size() != 0 {
		t.Errorf("after . on absent today: journal should be empty; got %d bytes", info.Size())
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("after . on absent today: journal mode: want 0o644, got %v", info.Mode().Perm())
	}

	// Now `e` is reachable. Dispatching it should open the in-app editor.
	pressE(t, a)
	if a.editor == nil {
		t.Errorf("after . created journal, e should open the in-app editor")
	}
}

// TestShiftE_EnsureFileStillUsed: editCurrent's `t0.IsZero()` branch
// is a defensive fallback for the narrow race where a file is
// deleted between boot and `E`. The journal-bootstrap branch above
// fires first, so the `t0.IsZero()` branch is only reachable for
// non-journal pages — a case that can't normally occur (the index
// only lists existing files). We don't have a realistic setup for
// that race, so this test stays as a smoke check that the no-op
// path doesn't crash.
func TestShiftE_EnsureFileStillUsed(t *testing.T) {
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/Alpha.md": "content\n"})
	a := New(dir, "test")
	cmd := a.Init()
	if cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("Alpha")

	// `E` on an existing Alpha.md should return a non-nil cmd.
	if cmd := pressShiftE(t, a); cmd == nil {
		t.Errorf("E on existing Alpha.md should return a non-nil cmd; got nil")
	}
}

var readLineRe = regexp.MustCompile(`line \d+`)

// cursorReadLine returns the "line N" text on the read view's cursor row.
func cursorReadLine(t *testing.T, a *App) string {
	t.Helper()
	row := a.page.rowText(a.page.CursorRow())
	m := readLineRe.FindString(row)
	if m == "" {
		t.Fatalf("no 'line N' on read cursor row %d: %q", a.page.CursorRow(), row)
	}
	return m
}

func bullets(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "- line %d\n", i)
	}
	return b.String()
}

func assertEditorOnCursorRow(t *testing.T, raw string) {
	t.Helper()
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/Long.md": raw})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	a.navigate("Long")
	for range 30 {
		a.page.LineDown()
	}
	want := "- " + cursorReadLine(t, a)
	wantY := screenRow(a.page)
	pressE(t, a)
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	line := cursorPos(a.editor).Line
	if line == 0 {
		t.Fatal("editor opened at line 0, want the reading position")
	}
	if cur := a.View().Cursor; cur == nil || cur.Y != wantY {
		t.Fatalf("cursor %+v, want it on the read cursor's screen row %d", cur, wantY)
	}
	if got := strings.Split(text(a.editor), "\n")[line]; got != want {
		t.Fatalf("editor cursor line = %q, want %q", got, want)
	}
	// The window must show the reading position, not just hold the cursor:
	// a line from above the cursor has to be on screen too.
	var n int
	fmt.Sscanf(want, "- line %d", &n)
	if above := fmt.Sprintf("line %d", n-5); wantY >= 5 && !strings.Contains(appText(a), above) {
		t.Fatalf("editor view does not show %q above the reading position:\n%s", above, appText(a))
	}
}

func TestE_OpensAtReadingPosition(t *testing.T) {
	assertEditorOnCursorRow(t, bullets(80))
}

func TestE_OpensAtReadingPositionAfterLeadingBlankLines(t *testing.T) {
	assertEditorOnCursorRow(t, "\n\n\n"+bullets(80))
}

// rowText is a frame row as compared across the read and edit views: trimmed,
// with the read view's bullet glyph mapped back to source "- " and the wiki
// link brackets (hidden syntax there) removed.
func rowText(row string) string {
	row = strings.NewReplacer("[[", "", "]]", "").Replace(strings.TrimSpace(row))
	for _, glyph := range []string{"• ", "* "} {
		if strings.HasPrefix(row, glyph) {
			return "- " + strings.TrimPrefix(row, glyph)
		}
	}
	return row
}

// assertEditorKeepsScreenRow presses e on a booted app and checks screen-row
// parity: the anchor row the read view showed on screen row ScreenRow is on
// the same screen row of the editor frame, with the same text, and the cursor
// sits on it.
func assertEditorKeepsScreenRow(t *testing.T, a *App) Anchor {
	t.Helper()
	at, ok := a.page.ReadingAnchor()
	if !ok {
		t.Fatal("no reading anchor")
	}
	read := strings.Split(appText(a), "\n")
	pressE(t, a)
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	edit := strings.Split(appText(a), "\n")
	if want, got := rowText(read[at.ScreenRow]), rowText(edit[at.ScreenRow]); got != want {
		t.Errorf("screen row %d: editor shows %q, read view showed %q\nread:\n%s\nedit:\n%s",
			at.ScreenRow, got, want, strings.Join(read, "\n"), strings.Join(edit, "\n"))
	}
	if cur := a.View().Cursor; cur == nil || cur.Y != at.ScreenRow {
		t.Errorf("cursor %+v, want it on screen row %d", cur, at.ScreenRow)
	}
	return at
}

func bootPage(t *testing.T, files map[string]string, page string, width, height int) *App {
	t.Helper()
	quietTerm(t)
	dir, _ := writeGraph(t, files)
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	a.navigate(page)
	return a
}

// A list-first page shows line 0 on read row 2; the editor keeps it there and
// puts the cursor on the first content line past the file's leading blanks.
func TestE_OnUnscrolledPageKeepsScreenRow(t *testing.T) {
	a := bootPage(t, map[string]string{"pages/Long.md": "\n\n- line 0\n- line 1\n"}, "Long", 80, 12)
	at := assertEditorKeepsScreenRow(t, a)
	if at != (Anchor{0, 0, 2}) {
		t.Errorf("anchor = %+v, want {0 0 2}", at)
	}
	if got := cursorPos(a.editor).Line; got != 2 {
		t.Fatalf("editor line = %d, want 2 (the first content line)", got)
	}
}

func TestE_ScrolledIntoWrappedBulletKeepsScreenRow(t *testing.T) {
	var b strings.Builder
	for i := range 30 {
		fmt.Fprintf(&b, "- %d %s\n", i, strings.Repeat("word ", 18))
	}
	a := bootPage(t, map[string]string{"pages/Long.md": b.String()}, "Long", 40, 12)
	// Move the cursor onto a continuation row of a wrapped bullet.
	for range 25 {
		a.page.LineDown()
	}
	if strings.HasPrefix(rowText(plain(a.page.rowText(a.page.CursorRow()))), "- ") {
		a.page.LineDown()
	}
	at := assertEditorKeepsScreenRow(t, a)
	if at.RowInLine == 0 {
		t.Errorf("anchor %+v: want a continuation row of the wrapped bullet", at)
	}
}

func TestE_VisibleLinkCursorKeepsScreenRow(t *testing.T) {
	a := bootPage(t, map[string]string{
		"pages/A.md": "- a\n- b\n- c\n- see [[B]]\n- d\n",
		"pages/B.md": "- b\n",
	}, "A", 80, 12)
	a.page.CycleLink(+1)
	at := assertEditorKeepsScreenRow(t, a)
	if at.Line != 3 {
		t.Errorf("anchor line = %d, want the link's line 3", at.Line)
	}
}

func TestE_LeadingBlankLinesKeepScreenRow(t *testing.T) {
	a := bootPage(t, map[string]string{"pages/Long.md": "\n\n\n" + bullets(80)}, "Long", 80, 12)
	for range 30 {
		a.page.LineDown()
	}
	at := assertEditorKeepsScreenRow(t, a)
	if got := cursorPos(a.editor).Line; got != at.Line+3 {
		t.Errorf("editor line = %d, want body line %d plus 3 leading blank lines", got, at.Line)
	}
}

func TestE_LogbookAboveTopRowKeepsScreenRow(t *testing.T) {
	a := bootPage(t, map[string]string{
		"pages/A.md": anchorLongPage("- head\n  :LOGBOOK:\n  CLOCK: x\n  :END:\n"),
	}, "A", 80, 10)
	scrollDown(a.page, 20)
	want := screenRow(a.page)
	at := assertEditorKeepsScreenRow(t, a)
	if at.ScreenRow != want {
		t.Errorf("anchor %+v, want the cursor's screen row %d", at, want)
	}
}

// pressDown moves the editor cursor one line down.
func pressDown(a *App) { a.Update(tea.KeyPressMsg{Code: tea.KeyDown}) }

func TestEOpensOnCursorRowLine(t *testing.T) {
	a := bootPage(t, map[string]string{"pages/Long.md": bullets(40)}, "Long", 80, 12)
	for range 4 {
		a.Update(key("j"))
	}
	want := screenRow(a.page)
	pressE(t, a)
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	if got := strings.Split(text(a.editor), "\n")[cursorPos(a.editor).Line]; got != "- line 4" {
		t.Fatalf("editor cursor line = %q, want %q", got, "- line 4")
	}
	if cur := a.View().Cursor; cur == nil || cur.Y != want {
		t.Fatalf("cursor %+v, want it on the read cursor's screen row %d", cur, want)
	}
}

func TestEscPutsCursorOnEditorLine(t *testing.T) {
	a := bootPage(t, map[string]string{"pages/Long.md": bullets(40)}, "Long", 80, 12)
	for range 4 {
		a.Update(key("j"))
	}
	pressE(t, a)
	for range 3 {
		pressDown(a)
	}
	y := a.View().Cursor.Y
	a.Update(esc)
	if a.editor != nil {
		t.Fatal("Esc did not close the editor")
	}
	if got := rowText(strings.Split(appText(a), "\n")[y]); got != "- line 7" {
		t.Fatalf("screen row %d shows %q, want the editor's line %q", y, got, "- line 7")
	}
	if got := screenRow(a.page); got != y {
		t.Fatalf("read cursor on screen row %d, want the editor's row %d", got, y)
	}
	if got := rowText(plain(a.page.rowText(a.page.CursorRow()))); got != "- line 7" {
		t.Fatalf("read cursor row shows %q, want %q", got, "- line 7")
	}
}

// A cursor on a row no source line spells (a table's top border, borrowed
// from the header below it) comes back exactly there after e, Esc: placing the
// anchor would move it onto the header row.
func TestEscUnmovedRestoresExactFrame(t *testing.T) {
	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	var a *App
	var rule int
	sourceRowsFor = func(body string, width int, emphasis string) ([]int, []bool) {
		lines, own := orig(body, width, emphasis)
		rows := a.page.full[:len(lines)]
		for rule = range rows { // a rule below the fold, treated as a border above the next line
			if strings.TrimSpace(plain(rows[rule])) == "--------" && rule > 20 {
				break
			}
		}
		header := rule + 1
		for !nonBlank(rows[header]) {
			header++
		}
		lines[rule], own[rule] = lines[header], false
		return lines, own
	}
	a = bootPage(t, map[string]string{"pages/Doc.md": rowlessPage()}, "Doc", 80, 20)
	a.page.sourceRows()
	for i := 0; a.page.CursorRow() != rule; i++ {
		if i > 300 {
			t.Fatalf("j never reached the borrowed row %d", rule)
		}
		a.Update(key("j"))
	}
	before := a.View().Content
	pressE(t, a)
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	a.Update(esc)
	if a.editor != nil {
		t.Fatal("Esc did not close the editor")
	}
	if got := a.View().Content; got != before {
		t.Fatalf("frame after e, Esc differs from before:\n%s\nwant:\n%s", plain(got), plain(before))
	}
}
