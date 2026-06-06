package views

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

// pressE sends the e key to a booted app and returns the resulting cmd.
func pressE(t *testing.T, a *App) tea.Cmd {
	t.Helper()
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	return cmd
}

// TestEditKey_DispatchesToCmd sends the e key on the page view and
// asserts that a non-nil cmd is returned. The cmd is the
// tea.ExecProcess wrapper; we don't run it (no real TTY here), we just
// confirm the dispatch fires.
func TestEditKey_DispatchesToCmd(t *testing.T) {
	a := bootApp(t)
	// Move to Alpha so we have a real file to target.
	a.navigate("Alpha")
	if a.page.Page() != "Alpha" {
		t.Fatalf("expected to land on Alpha, got %q", a.page.Page())
	}
	cmd := pressE(t, a)
	if cmd == nil {
		t.Errorf("e key should return a tea.ExecProcess cmd; got nil")
	}
}

// TestEditorExitedMsg_NoReindexOnNoChange constructs the message that
// tea.ExecProcess would yield when the child exits cleanly without
// changing the file. The handler should return (nil cmd) — no reindex.
func TestEditorExitedMsg_NoReindexOnNoChange(t *testing.T) {
	a := bootApp(t)
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	// Any existing fixture page is fine — we never actually call the
	// editor. Alpha.md is in the fixture.
	path := filepath.Join(abs, "pages", "Alpha.md")
	t0, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat Alpha.md: %v", err)
	}

	_, cmd := a.Update(editorExitedMsg{path: path, t0: t0.ModTime(), err: nil})
	if cmd != nil {
		t.Errorf("unchanged file should not reindex; got non-nil cmd")
	}
}

// TestEditorExitedMsg_TriggersReindexOnMtimeChange: the file's mtime
// has advanced; the handler should return a non-nil cmd (the
// buildIndexCmd).
func TestEditorExitedMsg_TriggersReindexOnMtimeChange(t *testing.T) {
	a := bootApp(t)
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(abs, "pages", "Alpha.md")

	// Pick a t0 well before the real mtime so the comparison advances.
	t0 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	_, cmd := a.Update(editorExitedMsg{path: path, t0: t0, err: nil})
	if cmd == nil {
		t.Errorf("mtime change should trigger reindex; got nil cmd")
	}
}

// TestEditorExitedMsg_NoReindexOnDelete: post-exit stat returns
// ENOENT — the user deleted the file in the editor. Silent no-op.
func TestEditorExitedMsg_NoReindexOnDelete(t *testing.T) {
	a := bootApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "deleted.md")
	// Don't create the file — os.Stat will return ENOENT.
	_, cmd := a.Update(editorExitedMsg{path: path, t0: time.Time{}, err: nil})
	if cmd != nil {
		t.Errorf("deleted file should not reindex; got non-nil cmd")
	}
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

// TestDotKey_CreatesMissingTodayJournal_ThenEditReachable: end-to-end
// for the new user flow. Boot with a journal file absent, press `.`
// to create-and-navigate, then press `e` to verify the page is now
// in the index and the editor cmd fires.
func TestDotKey_CreatesMissingTodayJournal_ThenEditReachable(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "journals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "Anchor.md"), []byte("anchor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No journal file written — today's journal is missing on disk.

	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New(dir, "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }
	// Drive the boot synchronously.
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

	// Now `e` is reachable. Dispatching it should return a non-nil
	// cmd (the tea.ExecProcess wrapper).
	if cmd := pressE(t, a); cmd == nil {
		t.Errorf("after . created journal, e should return a non-nil cmd; got nil")
	}
}

// TestEditKey_EnsureFileStillUsed: editCurrent's `t0.IsZero()` branch
// still calls edit.EnsureFile when the page is in the index but the
// file is missing on disk. This is reachable if a user (or external
// tool) deletes a file between the `.` press and the `e` press. We
// simulate the pre-condition via the same seed+remove dance the
// earlier version of this test used; the assertion is that the file
// is re-created empty.
func TestEditKey_EnsureFileStillUsed(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "journals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "Anchor.md"), []byte("anchor\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New(dir, "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }
	cmd := a.Init()
	if cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	today := "2026-06-05"
	journalPath := filepath.Join(dir, "journals", graph.FilenameFromPageName(today))
	// Seed the file so BuildIndex picks the journal up, then remove
	// it so the `e`-time SnapshotMtime returns zero and editCurrent
	// re-creates it.
	if err := os.WriteFile(journalPath, []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.idx = nil
	var idxErr error
	if a.idx, idxErr = graph.BuildIndex(dir); idxErr != nil {
		t.Fatal(idxErr)
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if _, ok := a.idx.ByName[today]; !ok {
		t.Fatalf("precondition: today journal must be in index")
	}
	if err := os.Remove(journalPath); err != nil {
		t.Fatalf("remove seed: %v", err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: journal file should not exist; stat err=%v", err)
	}

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	info, err := os.Stat(journalPath)
	if err != nil {
		t.Fatalf("journal file should have been recreated; stat err=%v", err)
	}
	if info.Size() != 0 {
		t.Errorf("journal file should be empty; got %d bytes", info.Size())
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("journal file mode: want 0o644, got %v", info.Mode().Perm())
	}
}
