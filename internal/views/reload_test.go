package views

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/layout"
	syncpkg "github.com/fiatcode-gh/weft/v2/internal/sync"
)

var oldInstant = time.Unix(1_000_000_000, 0)

// instantPolls makes every re-armed poll tick fire immediately so tests drive
// the loop by hand, without a clock.
func instantPolls(a *App) {
	a.pollTick = func() tea.Cmd { return func() tea.Msg { return pollTickMsg{} } }
}

// poll runs one tick of the poll loop to completion and reports whether it
// started (and finished) a rebuild. A tick that was paused returns false.
func poll(t *testing.T, a *App) bool {
	t.Helper()
	_, cmd := a.Update(pollTickMsg{})
	msg := cmd()
	if _, paused := msg.(pollTickMsg); paused {
		return false
	}
	done, ok := msg.(pollDoneMsg)
	if !ok {
		t.Fatalf("poll produced %T, want pollDoneMsg or pollTickMsg", msg)
	}
	gen := a.indexGen
	_, cmd = a.Update(done)
	if a.indexGen == gen {
		return false
	}
	a.Update(drainFor[indexLoadedMsg](t, cmd))
	return true
}

// quietApp boots a fixture clone with instant polls and proves a fresh boot
// does not trigger a rebuild.
func quietApp(t *testing.T) *App {
	t.Helper()
	a := bootApp(t)
	instantPolls(a)
	if poll(t, a) {
		t.Fatal("a freshly booted graph must not rebuild on the first poll")
	}
	return a
}

// expectOneRebuild asserts the pending outside change causes exactly one
// rebuild, then silence.
func expectOneRebuild(t *testing.T, a *App) {
	t.Helper()
	expectOneRebuildKeeping(t, a, func() {})
}

// expectOneRebuildKeeping is expectOneRebuild with a check between the
// rebuild and the quiet poll.
func expectOneRebuildKeeping(t *testing.T, a *App, check func()) {
	t.Helper()
	if !poll(t, a) {
		t.Fatal("first poll after an outside change must rebuild")
	}
	check()
	if poll(t, a) {
		t.Fatal("second poll must be quiet")
	}
}

func outsideWrite(t *testing.T, a *App, rel, body string) string {
	t.Helper()
	p := filepath.Join(a.graphPath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func chtimes(t *testing.T, p string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func resolves(a *App, name string) bool {
	_, ok := a.idx.Resolve(name)
	return ok
}

func TestPollRebuildsOnAddedPage(t *testing.T) {
	a := quietApp(t)
	outsideWrite(t, a, "pages/Zeta.md", "# Zeta\n")
	expectOneRebuildKeeping(t, a, func() {
		if !resolves(a, "Zeta") {
			t.Error("Zeta should be indexed after the rebuild")
		}
	})
}

func TestPollRebuildsOnChangedSize(t *testing.T) {
	a := quietApp(t)
	p := filepath.Join(a.graphPath, "pages", "Hub.md")
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	outsideWrite(t, a, "pages/Hub.md", "# Hub\n\nmuch longer body than before, certainly\n")
	chtimes(t, p, info.ModTime())
	expectOneRebuild(t, a)
}

func TestPollRebuildsOnChangedMtime(t *testing.T) {
	a := quietApp(t)
	chtimes(t, filepath.Join(a.graphPath, "pages", "Hub.md"), oldInstant)
	expectOneRebuild(t, a)
}

func TestPollRebuildsOnRemovedPage(t *testing.T) {
	a := quietApp(t)
	if !resolves(a, "Orphan") {
		t.Fatal("fixture should have Orphan")
	}
	if err := os.Remove(filepath.Join(a.graphPath, "pages", "Orphan.md")); err != nil {
		t.Fatal(err)
	}
	expectOneRebuildKeeping(t, a, func() {
		if resolves(a, "Orphan") {
			t.Error("Orphan should be gone after the rebuild")
		}
	})
}

func TestPollRebuildsOnSubfolderPage(t *testing.T) {
	a := quietApp(t)
	outsideWrite(t, a, "pages/sub/Deep.md", "# Deep\n")
	expectOneRebuildKeeping(t, a, func() {
		if !resolves(a, "sub/Deep") {
			t.Error("sub/Deep should be indexed after the rebuild")
		}
	})
}

func TestPollIgnoresDotFolderAndNonMarkdown(t *testing.T) {
	a := quietApp(t)
	outsideWrite(t, a, "pages/.trash/X.md", "# X\n")
	outsideWrite(t, a, "pages/notes.txt", "not markdown\n")
	if poll(t, a) {
		t.Error("changes the index never sees must not rebuild")
	}
}

func TestPollNoRebuildAfterOwnWrites(t *testing.T) {
	t.Run("R", func(t *testing.T) {
		a := quietApp(t)
		outsideWrite(t, a, "pages/Zeta.md", "# Zeta\n")
		_, cmd := a.Update(key("R"))
		a.Update(drainFor[indexLoadedMsg](t, cmd))
		if poll(t, a) {
			t.Error("R already indexed the outside change")
		}
	})
	t.Run("capture", func(t *testing.T) {
		a := bootApp(t, bootConfig{now: capNow})
		instantPolls(a)
		a.Update(key("c"))
		typeText(a, "buy milk")
		a.Update(keyEnt)
		if poll(t, a) {
			t.Error("capture reindexes synchronously; the poll must be quiet")
		}
	})
	t.Run("save and exit", func(t *testing.T) {
		a := bootApp(t, bootConfig{files: map[string]string{journalPath: journalBody}})
		instantPolls(a)
		a.navigate(journalPage)
		a.Update(key("e"))
		if a.editor == nil {
			t.Fatal("editor did not open")
		}
		typeApp(a, "X")
		a.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		_, cmd := a.Update(key("s"))
		a.Update(drainFor[indexLoadedMsg](t, cmd))
		if poll(t, a) {
			t.Error("save-and-exit already reindexed the write")
		}
	})
}

func TestPollPausedWhileSyncing(t *testing.T) {
	a := quietApp(t)
	a.statusProbe = func(string) (syncpkg.WorktreeStatus, error) { return syncpkg.WorktreeStatus{}, nil }
	a.syncing = true
	outsideWrite(t, a, "pages/Zeta.md", "# Zeta\n")
	gen := a.indexGen
	if poll(t, a) || a.indexGen != gen {
		t.Fatal("a tick during a sync must not scan or rebuild")
	}

	// A walk dispatched just before the sync began starts no rebuild either.
	a.syncing = false
	_, cmd := a.Update(pollTickMsg{})
	done := cmd()
	a.syncing = true
	a.Update(done)
	if a.indexGen != gen {
		t.Fatal("a poll result delivered during a sync must not rebuild")
	}

	a.Update(syncDoneMsg{res: syncpkg.Result{}})
	if !poll(t, a) {
		t.Fatal("the change made during the sync must be seen afterwards")
	}
	if poll(t, a) {
		t.Fatal("and only once")
	}
}

func TestPollSkipsWhileBuildInFlight(t *testing.T) {
	a := quietApp(t)
	build := a.buildIndexCmd()
	gen := a.indexGen
	outsideWrite(t, a, "pages/Zeta.md", "# Zeta\n")
	if poll(t, a) || a.indexGen != gen {
		t.Fatal("a tick while a build is in flight must be skipped")
	}
	a.Update(build().(indexLoadedMsg))
	if poll(t, a) {
		t.Fatal("the in-flight build walked after the write; nothing left to do")
	}
}

func TestPollDropsResultFromOlderIndex(t *testing.T) {
	a := quietApp(t)
	outsideWrite(t, a, "pages/Zeta.md", "# Zeta\n")
	_, cmd := a.Update(pollTickMsg{})
	stale := cmd()
	outsideWrite(t, a, "pages/Eta.md", "# Eta\n")
	if err := a.reindex(); err != nil {
		t.Fatal(err)
	}
	gen := a.indexGen
	a.Update(stale)
	if a.indexGen != gen {
		t.Fatal("a result scanned against a replaced index must be discarded")
	}
}

func TestPollFailedRebuildDoesNotLoop(t *testing.T) {
	a := quietApp(t)
	outsideWrite(t, a, "pages/Zeta.md", "# Zeta\n")
	_, cmd := a.Update(pollTickMsg{})
	a.Update(cmd())
	a.Update(indexLoadedMsg{err: errors.New("x"), gen: a.indexGen})
	if !strings.Contains(a.hint, "reindex failed") {
		t.Errorf("hint = %q, want reindex failed", a.hint)
	}
	if poll(t, a) {
		t.Error("a failed poll-driven rebuild must not retry every interval")
	}
}

func TestPollRefreshKeepsPlaceAndShowsNoHint(t *testing.T) {
	a := quietApp(t)
	a.navigate("2026_03_15")
	a.Update(key("j"))
	a.Update(key("j"))
	row, off, hist := a.page.CursorRow(), a.page.Offset(), len(a.hist)

	appendTo(t, filepath.Join(a.graphPath, "journals", "2026_03_15.md"), "- appended outside weft\n")
	if !poll(t, a) {
		t.Fatal("poll must rebuild")
	}
	if !strings.Contains(appText(a), "appended outside weft") {
		t.Errorf("read view should show the new line:\n%s", appText(a))
	}
	if a.page.CursorRow() != row || a.page.Offset() != off {
		t.Errorf("place moved: row %d→%d offset %d→%d", row, a.page.CursorRow(), off, a.page.Offset())
	}
	if len(a.hist) != hist {
		t.Errorf("history length %d → %d", hist, len(a.hist))
	}
	if strings.Contains(a.hint, "reindexing") {
		t.Errorf("hint = %q; the refresh must be silent", a.hint)
	}
}

func TestPollKeepsOpenOverlayRows(t *testing.T) {
	a := quietApp(t)
	a.Update(ctrl('p'))
	picker, ok := a.active.(*Picker)
	if !ok {
		t.Fatalf("active = %T, want *Picker", a.active)
	}
	before := plain(picker.View())

	p := outsideWrite(t, a, "pages/Zeta.md", "# Zeta\n")
	chtimes(t, p, time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	if !poll(t, a) {
		t.Fatal("poll must rebuild")
	}
	if a.active != picker {
		t.Fatal("the open overlay must not be replaced")
	}
	if got := plain(picker.View()); got != before || strings.Contains(got, "Zeta") {
		t.Errorf("open picker rows changed:\n%s", got)
	}

	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	a.Update(ctrl('p'))
	if v := plain(a.active.(*Picker).View()); !strings.Contains(v, "Zeta") {
		t.Errorf("a reopened picker should list Zeta:\n%s", v)
	}
}

func TestPollArmsOnceOnFirstLoad(t *testing.T) {
	arm := func(a *App) *int {
		n := 0
		a.pollTick = func() tea.Cmd { n++; return nil }
		return &n
	}
	t.Run("first success", func(t *testing.T) {
		a := New(cloneFixtureGraph(t), layout.Default(), "test")
		n := arm(a)
		a.Update(a.Init()())
		a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		if *n != 1 {
			t.Fatalf("armed %d times after boot, want 1", *n)
		}
		_, cmd := a.Update(key("R"))
		a.Update(drainFor[indexLoadedMsg](t, cmd))
		if *n != 1 {
			t.Errorf("armed %d times after R, want 1", *n)
		}
	})
	t.Run("failed first load", func(t *testing.T) {
		a := New(cloneFixtureGraph(t), layout.Default(), "test")
		n := arm(a)
		a.Update(indexLoadedMsg{err: errors.New("boom"), gen: a.indexGen})
		if *n != 0 {
			t.Errorf("armed %d times after a failed load, want 0", *n)
		}
	})
}

const outsideText = "changed outside weft"

func editorApp(t *testing.T) (*App, string) {
	t.Helper()
	a, path := openJournal(t)
	instantPolls(a)
	return a, path
}

func TestEditorOutsideChangeShowsNotice(t *testing.T) {
	a, path := editorApp(t)
	typeApp(a, "X")
	content, dirty, disk := a.editor.Content(), a.editor.dirty(), a.editor.disk
	appendTo(t, path, "- agent\n")

	if !poll(t, a) {
		t.Fatal("outside change must rebuild")
	}

	if !strings.Contains(plain(a.editor.View()), outsideText) {
		t.Error("view lacks the outside-change notice")
	}
	if a.editor.Content() != content || a.editor.dirty() != dirty || a.editor.disk != disk {
		t.Error("reload must not touch buffer, dirty state or disk baseline")
	}
}

func TestEditorOutsideNoticeClearsOnSave(t *testing.T) {
	a, path := editorApp(t)
	typeApp(a, "X")
	appendTo(t, path, "- agent\n")
	poll(t, a)

	a.Update(ctrlS)
	v := plain(a.editor.View())
	if strings.Contains(v, outsideText) || !strings.Contains(v, "merged") {
		t.Errorf("after a merged save want the merged notice only, got %q", v)
	}

	a.Update(key("Y"))
	v = plain(a.editor.View())
	if strings.Contains(v, outsideText) || !strings.Contains(v, "^S save") {
		t.Errorf("after one more key want default hints, got %q", v)
	}
}

func TestEditorOutsideNoticeAfterOwnSaveIsQuiet(t *testing.T) {
	a, _ := editorApp(t)
	typeApp(a, "X")
	a.Update(ctrlS)

	if !poll(t, a) {
		t.Fatal("a plain save is not reflected in the index; one rebuild expected")
	}
	if strings.Contains(plain(a.editor.View()), outsideText) {
		t.Error("own save must not raise the notice")
	}
}

func TestEditorOutsideNoticeKeepsAfterClashKeep(t *testing.T) {
	clash := func(t *testing.T) *App {
		a, path := editorApp(t)
		typeApp(a, "X")
		writeOutside(t, path, "- ONE\n- two\n- three\n")
		poll(t, a)
		if !strings.Contains(plain(a.editor.View()), outsideText) {
			t.Fatal("notice missing before the clash prompt")
		}
		a.Update(ctrlS)
		if a.editor.mode != confirmingClash {
			t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
		}
		return a
	}
	t.Run("keep", func(t *testing.T) {
		a := clash(t)
		a.Update(key("k"))
		if !strings.Contains(plain(a.editor.View()), outsideText) {
			t.Error("notice must return after keep editing")
		}
	})
	t.Run("reload", func(t *testing.T) {
		a := clash(t)
		a.Update(key("r"))
		if strings.Contains(plain(a.editor.View()), outsideText) {
			t.Error("reload must clear the notice")
		}
	})
}

func TestEditorOutsideNoticeOtherFileQuiet(t *testing.T) {
	a, _ := editorApp(t)
	outsideWrite(t, a, "pages/Other.md", "# Other\n")

	if !poll(t, a) {
		t.Fatal("outside change must rebuild")
	}
	if strings.Contains(plain(a.editor.View()), outsideText) {
		t.Error("a different file must not raise the notice")
	}
}

func TestEditorCompletionSeesOutsidePage(t *testing.T) {
	a, _ := editorApp(t)
	outsideWrite(t, a, "pages/Zeta.md", "# Zeta\n")
	if !poll(t, a) {
		t.Fatal("outside change must rebuild")
	}

	typeApp(a, "[[Zet")

	found := false
	for _, c := range a.editor.completer.cands {
		if c.name == "Zeta" {
			found = true
		}
	}
	if !found {
		t.Errorf("Zeta missing from candidates %+v", a.editor.completer.cands)
	}
	if !strings.Contains(plain(a.editor.View()), "Zeta") {
		t.Error("strip does not show Zeta")
	}
}

func TestEditorReloadReadErrorKeepsNotice(t *testing.T) {
	a, path := editorApp(t)
	appendTo(t, path, "- agent\n")
	poll(t, a)
	if !strings.Contains(plain(a.editor.View()), outsideText) {
		t.Fatal("notice missing")
	}
	real := a.readSnapshot
	a.readSnapshot = func(p string) (edit.Snapshot, error) {
		if p == path {
			return edit.Snapshot{}, errors.New("boom")
		}
		return real(p)
	}
	appendTo(t, path, "- more\n")

	if !poll(t, a) {
		t.Fatal("second outside change must rebuild")
	}
	if !strings.Contains(plain(a.editor.View()), outsideText) {
		t.Error("a failed read must leave the notice as it was")
	}
}
