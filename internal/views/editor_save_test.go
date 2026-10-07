package views

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
)

var ctrlS = tea.KeyMsg{Type: tea.KeyCtrlS}

const (
	journalPath = "journals/2026_05_24.md"
	journalPage = "2026-05-24"
	journalBody = "- one\n- two\n- three\n"
)

// openEditor boots a throwaway graph, navigates to page and opens the
// in-app editor on it, returning the app and the editor's file path.
func openEditor(t *testing.T, files map[string]string, page string) (*App, string) {
	t.Helper()
	a := bootApp(t, bootConfig{files: files})
	a.navigate(page)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatalf("editor did not open on %q", page)
	}
	return a, a.editor.path
}

func openJournal(t *testing.T) (*App, string) {
	t.Helper()
	return openEditor(t, map[string]string{journalPath: journalBody}, journalPage)
}

func typeApp(a *App, s string) {
	for _, r := range s {
		a.Update(key(string(r)))
	}
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func writeOutside(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEditorSaveMergesOutsideAppend(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	appendTo(t, path, "- agent\n")

	a.Update(ctrlS)

	want := "X- one\n- two\n- three\n- agent\n"
	if got := readFile(t, path); got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
	if got := a.editor.Content(); got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if a.editor.dirty() {
		t.Error("buffer dirty after merged save")
	}
	if a.editor.mode != editing {
		t.Errorf("mode = %v, want editing", a.editor.mode)
	}
	if !strings.Contains(a.editor.View(), "merged") {
		t.Error("view lacks merged notice")
	}

	typeApp(a, "Y")
	a.Update(ctrlS)
	want = "XY- one\n- two\n- three\n- agent\n"
	if got := readFile(t, path); got != want {
		t.Errorf("second save file = %q, want %q", got, want)
	}
	if strings.Contains(a.editor.View(), "merged") {
		t.Error("notice should clear on the next key")
	}
}

func TestEditorSaveBothAppendAtEnd(t *testing.T) {
	a, path := openJournal(t)
	a.editor.ta.SetValue("- one\n- two\n- three\n- mine\n")
	appendTo(t, path, "- agent\n")

	a.Update(ctrlS)

	want := "- one\n- two\n- three\n- mine\n- agent\n"
	if a.editor.mode != editing {
		t.Fatalf("mode = %v, want editing (no prompt)", a.editor.mode)
	}
	v := a.editor.View()
	if strings.Contains(v, "on disk") || !strings.Contains(v, "merged") {
		t.Errorf("view should show merged notice only: %q", v)
	}
	if got := readFile(t, path); got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if got := a.editor.Content(); got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if a.editor.dirty() {
		t.Error("buffer dirty after merged save")
	}
}

func TestEditorSaveBothAppendToEmptyJournal(t *testing.T) {
	a, path := openEditor(t, map[string]string{journalPath: ""}, journalPage)
	if a.editor.disk != (edit.Snapshot{Content: "", Exists: true}) {
		t.Fatalf("disk = %+v, want empty existing file", a.editor.disk)
	}
	typeApp(a, "- mine")
	appendTo(t, path, "- agent\n")

	a.Update(ctrlS)

	want := "- mine\n- agent\n"
	if a.editor.mode != editing {
		t.Fatalf("mode = %v, want editing (no prompt)", a.editor.mode)
	}
	if !strings.Contains(a.editor.View(), "merged") {
		t.Error("view lacks merged notice")
	}
	if got := readFile(t, path); got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if got := a.editor.Content(); got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if a.editor.ta.Line() != 0 {
		t.Errorf("cursor row = %d, want 0", a.editor.ta.Line())
	}
}

func TestEditorSaveMergeKeepsCursorOnSameText(t *testing.T) {
	a, path := openEditor(t, map[string]string{"pages/Lines.md": "l1\nl2\nl3\nl4\nl5\n"}, "Lines")
	for range 3 {
		a.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	typeApp(a, "X")
	writeOutside(t, path, "n1\nn2\nl1\nl2\nl3\nl4\nl5\n")

	a.Update(ctrlS)

	if a.editor.mode != editing {
		t.Fatalf("mode = %v, want editing", a.editor.mode)
	}
	if got := a.editor.ta.Line(); got != 5 {
		t.Errorf("cursor row = %d, want 5", got)
	}
	if before, _ := a.editor.cursorLineSplit(); before != "X" {
		t.Errorf("text before cursor = %q, want %q", before, "X")
	}
}

func TestEditorSaveOverlapShowsClashPrompt(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	theirs := "- ONE\n- two\n- three\n"
	writeOutside(t, path, theirs)
	buf := a.editor.Content()

	a.Update(ctrlS)

	if got := readFile(t, path); got != theirs {
		t.Fatalf("file = %q, want untouched %q", got, theirs)
	}
	if a.editor.mode != confirmingClash {
		t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
	}
	if !strings.Contains(a.editor.View(), "Changed on disk") {
		t.Error("view lacks clash prompt")
	}

	a.Update(key("k"))
	if a.editor.mode != editing || a.editor.Content() != buf {
		t.Errorf("k: mode=%v buffer=%q, want editing and unchanged buffer", a.editor.mode, a.editor.Content())
	}
	if got := readFile(t, path); got != theirs {
		t.Errorf("file after k = %q, want %q", got, theirs)
	}

	a.Update(ctrlS)
	if a.editor.mode != confirmingClash {
		t.Fatalf("second save: mode = %v, want confirmingClash", a.editor.mode)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.editor.mode != editing {
		t.Errorf("esc: mode = %v, want editing", a.editor.mode)
	}
}

func TestEditorSaveNewPageAppearedShowsClash(t *testing.T) {
	a, path := openEditor(t, map[string]string{"pages/Anchor.md": "- a\n"}, "Fresh")
	typeApp(a, "x")
	writeOutside(t, path, "- outside\n")

	a.Update(ctrlS)

	if got := readFile(t, path); got != "- outside\n" {
		t.Errorf("file = %q, want untouched", got)
	}
	if a.editor.mode != confirmingClash {
		t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
	}
	if !strings.Contains(a.editor.View(), "Changed on disk") {
		t.Error("view lacks clash prompt")
	}
}

func TestEditorSaveDeletedShowsDeletionPrompt(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	a.Update(ctrlS)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should stay absent, stat err = %v", err)
	}
	if a.editor.mode != confirmingClash {
		t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
	}
	if !strings.Contains(a.editor.View(), "Deleted on disk") {
		t.Error("view lacks deletion prompt")
	}
}

func TestEditorSaveMtimeOnlyChangeSavesPlainly(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	a.Update(ctrlS)

	if got, want := readFile(t, path), a.editor.Content(); got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	v := a.editor.View()
	if a.editor.mode != editing || strings.Contains(v, "merged") || strings.Contains(v, "on disk") {
		t.Errorf("mode=%v view=%q, want plain save", a.editor.mode, v)
	}
}

func TestEditorSaveAndExitClashStaysInEditor(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	theirs := "- ONE\n- two\n- three\n"
	writeOutside(t, path, theirs)

	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a.Update(key("s"))

	if a.editor == nil {
		t.Fatal("editor closed despite clash")
	}
	if a.editor.mode != confirmingClash {
		t.Errorf("mode = %v, want confirmingClash", a.editor.mode)
	}
	if got := readFile(t, path); got != theirs {
		t.Errorf("file = %q, want %q", got, theirs)
	}
}

func TestEditorSaveAndExitMergeExits(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	appendTo(t, path, "- agent\n")

	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a.Update(key("s"))

	if a.editor != nil {
		t.Fatal("editor should close after a merged save-and-exit")
	}
	if got, want := readFile(t, path), "X- one\n- two\n- three\n- agent\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if !strings.Contains(a.hint, "merged") {
		t.Errorf("hint = %q, want merged notice", a.hint)
	}
}

func TestEditorSaveUnloadableMergeIsClash(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	appendTo(t, path, "\tindented\n")
	theirs := readFile(t, path)

	a.Update(ctrlS)

	if a.editor.mode != confirmingClash {
		t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
	}
	if got := readFile(t, path); got != theirs {
		t.Errorf("file = %q, want untouched %q", got, theirs)
	}
}

func TestEditorSaveChangedWhileSavingKeepsBuffer(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	buf := a.editor.Content()
	real := a.readSnapshot
	raced := false
	a.readSnapshot = func(p string) (edit.Snapshot, error) {
		s, err := real(p)
		if p == path && !raced {
			raced = true
			writeOutside(t, path, "- raced\n")
		}
		return s, err
	}

	a.Update(ctrlS)

	if got := readFile(t, path); got != "- raced\n" {
		t.Errorf("file = %q, want raced content", got)
	}
	if a.editor.errMsg != changedWhileSavingMsg {
		t.Errorf("errMsg = %q, want %q", a.editor.errMsg, changedWhileSavingMsg)
	}
	if a.editor.Content() != buf {
		t.Errorf("buffer changed: %q", a.editor.Content())
	}
	if a.editor.saved {
		t.Error("editor marked saved after failed write")
	}
}

const clashTheirs = "- ONE\n- two\n- three\n"

// openClash opens the journal, types X and writes an overlapping outside
// change, then saves into the clash prompt.
func openClash(t *testing.T) (*App, string) {
	t.Helper()
	a, path := openJournal(t)
	typeApp(a, "X")
	writeOutside(t, path, clashTheirs)
	a.Update(ctrlS)
	if a.editor.mode != confirmingClash {
		t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
	}
	return a, path
}

func TestClashOverwriteWritesMine(t *testing.T) {
	a, path := openClash(t)

	a.Update(key("o"))

	if got, want := readFile(t, path), "X- one\n- two\n- three\n"; got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
	if a.editor.mode != editing || a.editor.dirty() || !a.editor.saved {
		t.Errorf("mode=%v dirty=%v saved=%v, want editing, clean, saved", a.editor.mode, a.editor.dirty(), a.editor.saved)
	}

	writeOutside(t, path, clashTheirs)
	a.Update(ctrlS)
	if a.editor.mode != editing {
		t.Errorf("mode after second save = %v, want plain save", a.editor.mode)
	}
}

func TestClashReloadTakesTheirs(t *testing.T) {
	a, path := openClash(t)

	a.Update(key("r"))

	if got := a.editor.Content(); got != clashTheirs {
		t.Fatalf("buffer = %q, want %q", got, clashTheirs)
	}
	if a.editor.dirty() {
		t.Error("buffer dirty after reload")
	}
	if got := readFile(t, path); got != clashTheirs {
		t.Errorf("file = %q, want %q", got, clashTheirs)
	}
	if a.editor.mode != editing {
		t.Fatalf("mode = %v, want editing", a.editor.mode)
	}

	typeApp(a, "Z")
	a.Update(ctrlS)
	if a.editor.mode != editing {
		t.Fatalf("mode after save = %v, want editing", a.editor.mode)
	}
	got := readFile(t, path)
	if got != a.editor.Content() || !strings.Contains(got, "Z") || !strings.Contains(got, "ONE") {
		t.Errorf("file = %q, want buffer %q containing Z and ONE", got, a.editor.Content())
	}
}

func TestClashKeepEditingChangesNothing(t *testing.T) {
	for name, k := range map[string]tea.KeyMsg{"k": key("k"), "esc": {Type: tea.KeyEsc}} {
		t.Run(name, func(t *testing.T) {
			a, path := openClash(t)

			a.Update(k)

			if got, want := a.editor.Content(), "X- one\n- two\n- three\n"; got != want {
				t.Errorf("buffer = %q, want %q", got, want)
			}
			if got := readFile(t, path); got != clashTheirs {
				t.Errorf("file = %q, want %q", got, clashTheirs)
			}
			if a.editor.mode != editing {
				t.Errorf("mode = %v, want editing", a.editor.mode)
			}
		})
	}
}

// A save error shown before the clash must not reappear once the user leaves
// the prompt: the clash supersedes it.
func TestClashClearsStaleSaveError(t *testing.T) {
	a, _ := openJournal(t)
	typeApp(a, "X")
	a.editor.SetError(changedWhileSavingMsg)
	writeOutside(t, a.editor.path, clashTheirs)
	a.Update(ctrlS)
	if a.editor.mode != confirmingClash {
		t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
	}

	a.Update(key("k"))

	if a.editor.errMsg != "" {
		t.Errorf("errMsg = %q after leaving the clash prompt, want empty", a.editor.errMsg)
	}
}

func TestClashDeletedOverwriteRecreates(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	buf := a.editor.Content()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	a.Update(ctrlS)

	a.Update(key("r"))

	if a.editor.mode != confirmingClash || a.editor.Content() != buf {
		t.Fatalf("after r: mode=%v buffer=%q, want clash prompt and intact buffer", a.editor.mode, a.editor.Content())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should stay absent, stat err = %v", err)
	}
	if strings.Contains(a.editor.View(), "[r]") {
		t.Error("deleted prompt must not offer reload")
	}

	a.Update(key("o"))

	if got := readFile(t, path); got != buf {
		t.Errorf("file = %q, want %q", got, buf)
	}
}

func TestClashNewPageOverwrite(t *testing.T) {
	a, path := openEditor(t, map[string]string{"pages/Anchor.md": "- a\n"}, "Fresh")
	typeApp(a, "x")
	writeOutside(t, path, "- outside\n")
	a.Update(ctrlS)
	if a.editor.mode != confirmingClash {
		t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
	}

	a.Update(key("o"))

	if got, want := readFile(t, path), a.editor.Content(); got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestClashSaveAndExitOverwriteExits(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	writeOutside(t, path, clashTheirs)
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a.Update(key("s"))
	if a.editor == nil || a.editor.mode != confirmingClash {
		t.Fatal("want clash prompt with editor open")
	}

	a.Update(key("o"))

	if a.editor != nil {
		t.Error("editor should close after overwrite from save-and-exit")
	}
	if got, want := readFile(t, path), "X- one\n- two\n- three\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestClashSaveAndExitReloadStays(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	writeOutside(t, path, clashTheirs)
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a.Update(key("s"))

	a.Update(key("r"))

	if a.editor == nil {
		t.Fatal("reload must not exit")
	}
	if a.editor.Content() != clashTheirs || a.editor.mode != editing {
		t.Errorf("buffer=%q mode=%v, want theirs and editing", a.editor.Content(), a.editor.mode)
	}
}

func TestClashOverwriteAfterFurtherChangeRefuses(t *testing.T) {
	a, path := openClash(t)
	writeOutside(t, path, "- third\n")

	a.Update(key("o"))

	if got := readFile(t, path); got != "- third\n" {
		t.Errorf("file = %q, want untouched", got)
	}
	if a.editor == nil {
		t.Fatal("editor closed")
	}
	if a.editor.errMsg != changedWhileSavingMsg || a.editor.mode != editing {
		t.Errorf("errMsg=%q mode=%v, want changed-while-saving and editing", a.editor.errMsg, a.editor.mode)
	}
}

func TestClashUnloadableTheirsOffersNoReload(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	writeOutside(t, path, "- ONE\tx\n- two\n- three\n")
	a.Update(ctrlS)
	if a.editor.mode != confirmingClash {
		t.Fatalf("mode = %v, want confirmingClash", a.editor.mode)
	}

	a.Update(key("r"))

	v := a.editor.View()
	if a.editor.mode != confirmingClash || !strings.Contains(v, "can't load it here") || strings.Contains(v, "[r]") {
		t.Fatalf("mode=%v view=%q, want clash prompt without reload", a.editor.mode, v)
	}

	a.Update(key("o"))

	if got, want := readFile(t, path), "X- one\n- two\n- three\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}
