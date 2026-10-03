package views

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
	syncpkg "git.fiatcode.dev/fiatcode/weft/v2/internal/sync"
)

// pressE sends the e key to a booted app and returns the resulting cmd.
func pressE(t *testing.T, a *App) tea.Cmd {
	t.Helper()
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	return cmd
}

// pressShiftE sends the E key (the $EDITOR escape hatch).
func pressShiftE(t *testing.T, a *App) tea.Cmd {
	t.Helper()
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	return cmd
}

func TestE_EntersInAppEditor(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	if cmd := pressE(t, a); cmd == nil {
		t.Errorf("e should return the textarea focus cmd; got nil")
	}
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

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
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
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
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
	if cmd := pressE(t, a); cmd == nil {
		t.Errorf("after . created journal, e should open the editor (non-nil focus cmd); got nil")
	}
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

func TestNewEditorViewFlagsSanitizerDivergence(t *testing.T) {
	// arrange
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"clean", "- a\n- b\n", false},
		{"no trailing newline", "- a", false},
		{"empty new page", "", false},
		{"crlf", "a\r\nb\r\n", true},
		{"tab indent", "code:\n\tindented\n", true},
		{"over textarea line cap", strings.Repeat("x\n", 10001), true},
	}
	for _, tc := range cases {
		for _, anchor := range []int{0, 2} {
			t.Run(fmt.Sprintf("%s/anchor %d", tc.name, anchor), func(t *testing.T) {
				// act
				e := NewEditorView(nil, "P", "unused.md", tc.content, false, 80, 24, anchor)

				// assert
				if got := e.LoadDiverged(); got != tc.want {
					t.Errorf("LoadDiverged = %v, want %v", got, tc.want)
				}
			})
		}
	}
}
