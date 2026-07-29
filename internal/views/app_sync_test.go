package views

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
	"git.fiatcode.dev/fiatcode/weft/v2/internal/search"
	syncpkg "git.fiatcode.dev/fiatcode/weft/v2/internal/sync"
)

var errSyncTest = errors.New("boom")

// drainFor runs cmd (and any tea.Batch children) concurrently until it
// produces a T. Mirrors the real Bubble Tea runtime which runs batch children
// concurrently, so a slow sibling cmd (e.g. a 3-second tick) does not block
// the fast one from being returned.
func drainFor[T tea.Msg](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	found := make(chan T, 1)
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		msg := c()
		if m, ok := msg.(T); ok {
			select {
			case found <- m:
			default:
			}
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				go run(child)
			}
		}
	}
	go run(cmd)
	select {
	case m := <-found:
		return m
	case <-time.After(5 * time.Second):
		var zero T
		t.Fatalf("cmd did not produce %T within timeout", zero)
		return zero
	}
}

func TestSyncKeyShowsSyncingHintAndRunsRunner(t *testing.T) {
	a := bootApp(t)
	called := false
	a.syncFunc = func(repoDir string) syncpkg.Result {
		called = true
		return syncpkg.Result{Pushed: true}
	}
	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	a = model.(*App)
	if !strings.Contains(a.hint, "syncing") {
		t.Errorf("hint = %q, want syncing…", a.hint)
	}
	if !a.syncing {
		t.Errorf("expected syncing flag set")
	}
	// Draining the returned cmd runs the runner and yields syncDoneMsg.
	msg := drainFor[syncDoneMsg](t, cmd)
	if !called {
		t.Errorf("runner was not invoked")
	}
	model, _ = a.Update(msg)
	a = model.(*App)
	if a.syncing {
		t.Errorf("syncing flag should clear on done")
	}
	if !strings.Contains(a.hint, "synced") {
		t.Errorf("hint = %q, want ✓ synced", a.hint)
	}
}

func TestSyncSecondPressWhileSyncingIsNoOp(t *testing.T) {
	a := bootApp(t)
	a.syncing = true
	calls := 0
	a.syncFunc = func(string) syncpkg.Result { calls++; return syncpkg.Result{} }
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	a = model.(*App)
	if calls != 0 {
		t.Errorf("runner should not be called while syncing")
	}
	if !strings.Contains(a.hint, "already syncing") {
		t.Errorf("hint = %q, want already syncing", a.hint)
	}
}

func TestSyncPulledTriggersReindex(t *testing.T) {
	a := bootApp(t)
	model, cmd := a.Update(syncDoneMsg{res: syncpkg.Result{Pushed: true, Pulled: true}})
	a = model.(*App)
	if !strings.Contains(a.hint, "synced") {
		t.Errorf("hint = %q", a.hint)
	}
	// Prove that buildIndexCmd() was wired into the Pulled branch by draining
	// for the indexLoadedMsg it produces. With concurrent drainFor this is fast
	// even though the batch also contains a 3s setHint tick.
	drainFor[indexLoadedMsg](t, cmd)
}

func TestSyncFailureHintNamesStage(t *testing.T) {
	// arrange: a private cache dir — the sync-failure arm writes weft.log.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := bootApp(t)
	model, _ := a.Update(syncDoneMsg{res: syncpkg.Result{Stage: "push", Output: "rejected", Err: errSyncTest}})
	a = model.(*App)
	if !strings.Contains(a.hint, "push failed") {
		t.Errorf("hint = %q, want push failed", a.hint)
	}
}

func TestWriteKeysBlockedWhileSyncing(t *testing.T) {
	for _, key := range []string{"e", "E", "."} {
		t.Run(key, func(t *testing.T) {
			// arrange
			a := bootApp(t)
			a.syncing = true

			// act
			model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			a = model.(*App)

			// assert
			if a.editor != nil {
				t.Fatal("editor opened during sync")
			}
			if !strings.Contains(a.hint, "sync in progress") {
				t.Fatalf("hint = %q, want sync-in-progress hint", a.hint)
			}
		})
	}
}

// TestLinkifyBlockedWhileSyncing covers the linkify outcome dispatch branch in
// App.Update (the "y confirm" step of the Backlinks overlay), the
// highest-risk of the sync-busy-gated entry points because it writes
// straight to a source file on disk rather than through the editor's own
// save path.
//
// The real key sequence to reach this branch is "b" (open backlinks) then
// "l" then "y" on an unlinked row, but opening the overlay via "b" shells
// out to ripgrep (App.unlinkedRefs), so — mirroring the existing pattern in
// backlinks_test.go's unlinkedFixture/selectFirstUnlinked helpers — this
// test builds the Backlinks overlay directly with a synthetic UnlinkedRef
// pointing at a real file, drives it into the confirm sub-state with its
// own real Update("l"), and only then hands the "y" tea.KeyMsg to a.Update.
// That still exercises the actual guarded branch (a.active.Update(key)
// producing an overlayLinkify result, gated by blockIfSyncing) rather than
// calling a.linkify directly, which would bypass the guard entirely.
func TestLinkifyBlockedWhileSyncing(t *testing.T) {
	// arrange
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{
		"pages/Hub.md":  "# Hub\n",
		"pages/Beta.md": "intro\na bare Hub mention\n",
	})
	a := New(dir, "test")
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	a.Update(cmd())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}

	betaPath := filepath.Join(dir, "pages", "Beta.md")
	before, err := os.ReadFile(betaPath)
	if err != nil {
		t.Fatal(err)
	}

	unl := []graph.UnlinkedRef{{
		FilePath: betaPath,
		PageName: "Beta",
		Line:     2,
		Context:  "a bare Hub mention",
		Match:    search.Span{Start: 7, End: 10},
	}}
	b := NewBacklinks(a.idx, "Hub", unl, a.width, a.height)
	selectFirstUnlinked(t, b)
	b.Update("l") // real transition into the confirm sub-state
	a.active = b
	a.syncing = true

	// act
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	// assert
	after, err := os.ReadFile(betaPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("Beta.md mutated while syncing: before=%q after=%q", before, after)
	}
	out := b.View()
	if !strings.Contains(out, "sync in progress") {
		t.Fatalf("expected in-panel busy message, got:\n%s", out)
	}
	// The busy message renders verbatim (matching the picker) — it must not
	// read as a self-contradictory "linkify failed: sync in progress".
	if strings.Contains(out, "linkify failed") {
		t.Fatalf("busy message should not be framed as a failure, got:\n%s", out)
	}
}

// stubOverlay drives App.Update's overlay-dispatch branch with a canned
// OverlayResult, standing in for a real picker/backlinks overlay.
type stubOverlay struct{ res OverlayResult }

func (s stubOverlay) Update(string) OverlayResult { return s.res }
func (s stubOverlay) View() string                { return "" }
func (s stubOverlay) SetSize(int, int)            {}

// A blocked create must say so inside the picker: the status bar is hidden
// behind the overlay, so a hint there reads as a dead keypress.
func TestPickerCreateBlockedWhileSyncingShowsInPanelMessage(t *testing.T) {
	// arrange
	quietTerm(t)
	a := bootApp(t)
	a.syncing = true
	p := NewPicker(a.idx, 80, 24)
	typeQuery(p, "Brand New Page")
	if p.createName == "" {
		t.Fatal("arrange failed: expected a create row")
	}
	for p.sel < len(p.matches) { // move selection onto the create row
		p.moveDown(p.rowCount())
	}
	a.active = p

	// act
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// assert
	if a.active == nil {
		t.Fatal("picker should stay open when create is blocked")
	}
	if a.editor != nil {
		t.Fatal("editor must not open while syncing")
	}
	if !strings.Contains(p.View(), "sync in progress") {
		t.Fatalf("expected in-panel busy message, got:\n%s", p.View())
	}
}

func TestPickerCreateBlockedWhileSyncing(t *testing.T) {
	// arrange: a sync in flight and a picker about to create a page.
	a := bootApp(t)
	a.syncing = true
	a.active = stubOverlay{res: overlayCreate("Brand New")}

	// act
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(*App)

	// assert: no editor mid-rebase, same contract as e/E/./linkify.
	if a.editor != nil {
		t.Fatal("create opened the editor during a sync")
	}
	if !strings.Contains(a.hint, "sync in progress") {
		t.Errorf("hint = %q, want sync-in-progress", a.hint)
	}
}

func TestSyncFailureLogIncludesError(t *testing.T) {
	// arrange: a private cache dir so the test owns weft.log.
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	a := bootApp(t)

	// act: a push killed by timeout — empty Output, the cause only in Err.
	a.Update(syncDoneMsg{res: syncpkg.Result{Stage: "push", Output: "", Err: errSyncTest}})

	// assert
	b, err := os.ReadFile(filepath.Join(cache, "weft", "weft.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(b), errSyncTest.Error()) {
		t.Errorf("log %q does not record the sync error", string(b))
	}
}

func TestSyncFailureHintFallsBackInlineWhenLogUnwritable(t *testing.T) {
	// arrange: make the log path unopenable — a directory where the
	// file should be — without touching the checkout's cwd.
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	if err := os.MkdirAll(filepath.Join(cache, "weft", "weft.log"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := bootApp(t)

	// act
	model, _ := a.Update(syncDoneMsg{res: syncpkg.Result{Stage: "push", Err: errSyncTest}})
	a = model.(*App)

	// assert: the hint must carry the error itself, not point at a log
	// that was never written.
	if !strings.Contains(a.hint, errSyncTest.Error()) {
		t.Errorf("hint = %q, want inline error", a.hint)
	}
}

func TestEditorErrorExitStillReindexesChangedFile(t *testing.T) {
	// arrange: an indexed page whose on-disk mtime differs from the
	// snapshot taken at editor launch — i.e. the editor wrote the file.
	a := bootApp(t)
	path := a.idx.Pages[0].Path
	t0 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	// act: the editor exits non-zero (vim :cq after :w).
	model, cmd := a.Update(editorExitedMsg{path: path, t0: t0, err: errSyncTest})
	a = model.(*App)

	// assert: the exit is surfaced AND the reindex still runs.
	if !strings.Contains(a.hint, "editor exited") {
		t.Errorf("hint = %q, want editor-exited", a.hint)
	}
	drainFor[indexLoadedMsg](t, cmd)
}

func TestStatusProbeErrorKeepsLastKnownIndicator(t *testing.T) {
	// arrange: last successful probe said "unsynced".
	a := bootApp(t)
	a.unsynced = true

	// act: a probe failure says nothing about actual sync state.
	model, _ := a.Update(statusProbedMsg{err: errSyncTest})
	a = model.(*App)

	// assert
	if !a.unsynced {
		t.Error("probe error cleared the unsynced indicator to a false all-clear")
	}
}
