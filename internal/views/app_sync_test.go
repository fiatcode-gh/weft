package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
