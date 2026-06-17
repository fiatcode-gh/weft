package views

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	syncpkg "git.fiatcode.dev/fiatcode/weft/v2/internal/sync"
)

// stubProbe makes a's status probe return a fixed result without shelling out
// to git, then runs the probe and feeds its message back through Update — the
// production round-trip minus the tea runtime.
func stubProbe(t *testing.T, a *App, st syncpkg.WorktreeStatus, probeErr error) *App {
	t.Helper()
	a.statusProbe = func(string) (syncpkg.WorktreeStatus, error) { return st, probeErr }
	model, _ := a.Update(a.statusProbeCmd()())
	return model.(*App)
}

func TestStatusProbeDirtySetsDot(t *testing.T) {
	// Arrange
	a := bootApp(t)

	// Act: the probe reports the work tree dirty.
	a = stubProbe(t, a, syncpkg.WorktreeStatus{Dirty: true}, nil)

	// Assert
	if !a.unsynced {
		t.Fatal("a dirty probe should set unsynced")
	}
	if !strings.Contains(a.statusBar(), "●") {
		t.Errorf("status bar should show the dot when unsynced:\n%s", a.statusBar())
	}
}

func TestStatusProbeAheadSetsDot(t *testing.T) {
	// Arrange
	a := bootApp(t)

	// Act: the probe reports unpushed commits.
	a = stubProbe(t, a, syncpkg.WorktreeStatus{Ahead: 2}, nil)

	// Assert
	if !a.unsynced {
		t.Error("an ahead-of-upstream probe should set unsynced")
	}
}

func TestStatusProbeCleanHidesDot(t *testing.T) {
	// Arrange
	a := bootApp(t)

	// Act: the probe reports a clean, in-sync tree.
	a = stubProbe(t, a, syncpkg.WorktreeStatus{}, nil)

	// Assert
	if a.unsynced {
		t.Error("a clean probe should leave unsynced false")
	}
	if strings.Contains(a.statusBar(), "●") {
		t.Errorf("a clean status bar should not show the dot:\n%s", a.statusBar())
	}
}

func TestStatusProbeErrorHidesDot(t *testing.T) {
	// Arrange
	a := bootApp(t)

	// Act: a non-repo graph (or any probe error), even if the status looks dirty.
	a = stubProbe(t, a, syncpkg.WorktreeStatus{Dirty: true}, errors.New("not a repo"))

	// Assert
	if a.unsynced {
		t.Error("a probe error should leave unsynced false")
	}
}

func TestStatusDotHiddenWhileHintShown(t *testing.T) {
	// Arrange: an unsynced graph with a transient hint occupying the right side.
	a := bootApp(t)
	a.unsynced = true
	a.hint = "✓ synced"

	// Act
	bar := a.statusBar()

	// Assert
	if strings.Contains(bar, "●") {
		t.Errorf("the dot should be hidden while a hint occupies the right side:\n%s", bar)
	}
}

// TestEditorExitRefreshesIndicatorWhenUnchanged covers the $EDITOR path where
// editCurrent created a journal stub but the editor exited without modifying
// it: the file is unchanged (so no reindex), yet the new untracked stub must
// still refresh the indicator.
func TestEditorExitRefreshesIndicatorWhenUnchanged(t *testing.T) {
	// Arrange: a real file whose mtime we pass as t0, driving the handler's
	// "unchanged" branch; the stubbed probe reports dirty.
	a := bootApp(t)
	probed := false
	a.statusProbe = func(string) (syncpkg.WorktreeStatus, error) {
		probed = true
		return syncpkg.WorktreeStatus{Dirty: true}, nil
	}
	path := filepath.Join(t.TempDir(), "stub.md")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	model, cmd := a.Update(editorExitedMsg{path: path, t0: info.ModTime()})
	a = model.(*App)
	msg := drainFor[statusProbedMsg](t, cmd)
	model, _ = a.Update(msg)
	a = model.(*App)

	// Assert
	if !probed {
		t.Fatal("editor-exit (unchanged) should run the status probe")
	}
	if !a.unsynced {
		t.Error("probe reported dirty; unsynced should be set")
	}
}
