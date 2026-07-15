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

// TestLinkifyBlockedWhileSyncing covers the res.Linkify dispatch branch in
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
// producing OverlayResult.Linkify, gated by syncBusyHint) rather than
// calling a.linkify directly, which would bypass the guard entirely.
func TestLinkifyBlockedWhileSyncing(t *testing.T) {
	// arrange
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
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	a = model.(*App)

	// assert
	after, err := os.ReadFile(betaPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("Beta.md mutated while syncing: before=%q after=%q", before, after)
	}
	if !strings.Contains(a.hint, "sync in progress") {
		t.Fatalf("hint = %q, want sync-in-progress hint", a.hint)
	}
}
